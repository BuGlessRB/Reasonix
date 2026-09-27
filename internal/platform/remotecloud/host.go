// Package remotecloud keeps a signed-in Studio reachable through the Reasonix
// message relay. Payloads are encrypted between the controller and Studio; the
// relay sees only routing identities, capability scopes and ciphertext.
package remotecloud

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"reasonix/internal/platform/account"
)

const DefaultRelayURL = "wss://remote.reasonix.io"

const desktopResponseChunk = 24 << 10

type Status struct {
	DeviceID string `json:"deviceId,omitempty"`
	Name     string `json:"name,omitempty"`
	Online   bool   `json:"online"`
	Error    string `json:"error,omitempty"`
}

type hostState struct {
	status Status
}

type Host struct {
	client   *account.Client
	dialer   *websocket.Dialer
	relayURL string
	version  string
	name     string
	token    func() string
	tasks    TaskService
	desktop  DesktopService

	mu    sync.RWMutex
	state hostState
}

type TaskService interface {
	CloudTasks(context.Context) (any, error)
	CloudTask(context.Context, string) (any, error)
	CloudSubmit(context.Context, string, string, string) error
}

type DesktopRequest struct {
	Method string
	Path   string
	Body   []byte
}

type DesktopResponse struct {
	Status      int
	ContentType string
	ETag        string
	Body        []byte
}

type DesktopService interface {
	CloudDesktop(context.Context, DesktopRequest, string) (DesktopResponse, error)
}

func New(client *account.Client, dialer *websocket.Dialer, relayURL, version string, tasks ...TaskService) *Host {
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	if strings.TrimSpace(relayURL) == "" {
		relayURL = DefaultRelayURL
	}
	name, _ := os.Hostname()
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Reasonix Studio"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	host := &Host{
		client: client, dialer: dialer, relayURL: strings.TrimRight(relayURL, "/"),
		version: version, name: name, token: account.Token,
	}
	if len(tasks) > 0 {
		host.tasks = tasks[0]
		host.desktop, _ = tasks[0].(DesktopService)
	}
	return host
}

func (h *Host) Status() Status {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state.status
}

func (h *Host) publish(status Status) {
	h.mu.Lock()
	h.state.status = status
	h.mu.Unlock()
}

func (h *Host) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		token := strings.TrimSpace(h.token())
		if token == "" {
			h.publish(Status{})
			if !wait(ctx, time.Second) {
				return
			}
			continue
		}
		saved, private, err := h.ensureIdentity(ctx, token)
		if err == nil {
			err = h.connect(ctx, token, saved, private)
		}
		if ctx.Err() != nil {
			return
		}
		status := h.Status()
		wasOnline := status.Online
		status.Online = false
		status.Error = err.Error()
		h.publish(status)
		if wasOnline {
			backoff = time.Second
		}
		if !wait(ctx, backoff) {
			return
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (h *Host) ensureIdentity(ctx context.Context, token string) (*identity, *ecdh.PrivateKey, error) {
	user, err := h.client.Me(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	saved, err := loadIdentity()
	if err == nil && saved != nil && saved.OwnerID == user.ID {
		private, keyErr := privateKey(saved)
		if keyErr == nil && h.identityCurrent(ctx, token, saved) {
			return saved, private, nil
		}
		if keyErr == nil {
			return h.registerIdentity(ctx, token, user.ID, private)
		}
	}
	private, public, err := generatePrivateKey()
	if err != nil {
		return nil, nil, err
	}
	return h.registerIdentityWithPublic(ctx, token, user.ID, private, public)
}

func (h *Host) identityCurrent(ctx context.Context, token string, saved *identity) bool {
	devices, err := h.client.RemoteDevices(ctx, token)
	if err != nil {
		return true
	}
	for _, device := range devices {
		if device.ID == saved.DeviceID {
			return slices.Contains(device.Capabilities, account.RemoteDesktop)
		}
	}
	return false
}

func (h *Host) registerIdentity(ctx context.Context, token string, ownerID int64, private *ecdh.PrivateKey) (*identity, *ecdh.PrivateKey, error) {
	public := base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes())
	return h.registerIdentityWithPublic(ctx, token, ownerID, private, public)
}

func (h *Host) registerIdentityWithPublic(ctx context.Context, token string, ownerID int64, private *ecdh.PrivateKey, public string) (*identity, *ecdh.PrivateKey, error) {
	registered, err := h.client.RegisterRemoteDevice(ctx, token, account.RemoteDeviceRegistration{
		Name: h.name, Platform: platformName(runtime.GOOS), PublicKey: public,
		Capabilities: []account.RemoteCapability{
			account.RemoteTasks, account.RemoteLogs, account.RemoteFiles, account.RemoteDesktop,
		},
	})
	if err != nil {
		return nil, nil, err
	}
	saved := &identity{
		OwnerID: ownerID, DeviceID: registered.Device.ID,
		DeviceCredential: registered.DeviceCredential,
		PrivateKey:       base64.RawURLEncoding.EncodeToString(private.Bytes()),
	}
	if err := saveIdentity(saved); err != nil {
		return nil, nil, err
	}
	return saved, private, nil
}

func platformName(goos string) string {
	if goos == "darwin" {
		return "macos"
	}
	return goos
}

type gatewayMessage struct {
	Type         string                     `json:"type"`
	ConnectionID string                     `json:"connectionId"`
	Scopes       []account.RemoteCapability `json:"scopes"`
	Payload      string                     `json:"payload"`
}

type controllerCommand struct {
	Version int    `json:"v"`
	Type    string `json:"type"`
	ID      string `json:"id"`
	TaskID  string `json:"taskId,omitempty"`
	Text    string `json:"text,omitempty"`
	Method  string `json:"method,omitempty"`
	Path    string `json:"path,omitempty"`
	Body    string `json:"body,omitempty"`
}

func (h *Host) connect(ctx context.Context, token string, saved *identity, private *ecdh.PrivateKey) error {
	url := h.relayURL + "/v1/devices/" + saved.DeviceID + "/connect"
	headers := http.Header{"Authorization": []string{"Bearer " + saved.DeviceCredential}}
	conn, response, err := h.dialer.DialContext(ctx, url, headers)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusUnauthorized {
			_ = clearIdentity()
		}
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(64 << 10)
	h.publish(Status{DeviceID: saved.DeviceID, Name: h.name, Online: true})

	done := make(chan struct{})
	defer close(done)
	messages := make(chan []byte, 1)
	readErr := make(chan error, 1)
	go func() {
		for {
			kind, payload, err := conn.ReadMessage()
			if err != nil {
				select {
				case readErr <- err:
				case <-done:
				}
				return
			}
			if kind != websocket.TextMessage {
				select {
				case readErr <- errors.New("remote cloud: relay sent a binary message"):
				case <-done:
				}
				return
			}
			select {
			case messages <- payload:
			case <-done:
				return
			}
		}
	}()

	sessions := make(map[string]*sessionCipher)
	pingTicker := time.NewTicker(20 * time.Second)
	tokenTicker := time.NewTicker(time.Second)
	defer pingTicker.Stop()
	defer tokenTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case <-tokenTicker.C:
			if strings.TrimSpace(h.token()) != token {
				return errors.New("remote cloud: account changed")
			}
		case <-pingTicker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return err
			}
		case payload := <-messages:
			if err := h.handle(ctx, conn, saved, private, sessions, payload); err != nil {
				continue
			}
		}
	}
}

func (h *Host) handle(
	ctx context.Context,
	conn *websocket.Conn,
	saved *identity,
	private *ecdh.PrivateKey,
	sessions map[string]*sessionCipher,
	payload []byte,
) error {
	var message gatewayMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return err
	}
	switch message.Type {
	case "controller_disconnected":
		delete(sessions, message.ConnectionID)
		return nil
	case "controller_connected":
		return nil
	case "controller_message":
	default:
		return errors.New("remote cloud: unknown relay message")
	}
	session := sessions[message.ConnectionID]
	if session == nil {
		created, err := newSessionCipher(private, saved.DeviceID, message.Payload)
		if err != nil {
			return err
		}
		sessions[message.ConnectionID] = created
		ready, err := created.seal(map[string]any{
			"v": protocolVersion, "type": "ready", "deviceId": saved.DeviceID,
			"name": h.name, "platform": platformName(runtime.GOOS), "version": h.version,
		})
		if err != nil {
			return err
		}
		return writeDirected(conn, message.ConnectionID, ready)
	}
	var command controllerCommand
	if err := session.open(message.Payload, &command); err != nil {
		return err
	}
	if command.Version != protocolVersion || command.ID == "" {
		return errors.New("remote cloud: unsupported controller command")
	}
	var response map[string]any
	switch command.Type {
	case "ping":
		response = map[string]any{"v": protocolVersion, "type": "pong", "id": command.ID, "at": time.Now().UTC().Format(time.RFC3339Nano)}
	case "tasks.list", "tasks.get", "tasks.send":
		response = h.taskCommand(ctx, "cloud:"+message.ConnectionID, message.Scopes, command)
	case "desktop.request":
		return h.desktopCommand(ctx, conn, session, "cloud:"+message.ConnectionID, message.ConnectionID, message.Scopes, command)
	default:
		return errors.New("remote cloud: unsupported controller command")
	}
	reply, err := session.seal(response)
	if err != nil {
		return err
	}
	return writeDirected(conn, message.ConnectionID, reply)
}

func (h *Host) desktopCommand(
	ctx context.Context,
	conn *websocket.Conn,
	session *sessionCipher,
	deviceID, connectionID string,
	scopes []account.RemoteCapability,
	command controllerCommand,
) error {
	if h.desktop == nil || !hasScope(scopes, account.RemoteDesktop) {
		return h.writeDesktopError(conn, session, connectionID, command.ID, "desktop access is unavailable")
	}
	body, err := base64.RawURLEncoding.DecodeString(command.Body)
	if err != nil {
		return h.writeDesktopError(conn, session, connectionID, command.ID, "desktop request body is invalid")
	}
	response, err := h.desktop.CloudDesktop(ctx, DesktopRequest{
		Method: command.Method, Path: command.Path, Body: body,
	}, deviceID)
	if err != nil {
		return h.writeDesktopError(conn, session, connectionID, command.ID, err.Error())
	}
	chunks := (len(response.Body) + desktopResponseChunk - 1) / desktopResponseChunk
	if chunks == 0 {
		chunks = 1
	}
	for index := range chunks {
		start := index * desktopResponseChunk
		end := min(start+desktopResponseChunk, len(response.Body))
		chunk := ""
		if start < len(response.Body) {
			chunk = base64.RawURLEncoding.EncodeToString(response.Body[start:end])
		}
		payload := map[string]any{
			"v": protocolVersion, "type": "desktop.response", "id": command.ID,
			"status": response.Status, "contentType": response.ContentType, "etag": response.ETag,
			"index": index, "done": index == chunks-1, "body": chunk,
		}
		reply, sealErr := session.seal(payload)
		if sealErr != nil {
			return sealErr
		}
		if writeErr := writeDirected(conn, connectionID, reply); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func (h *Host) writeDesktopError(conn *websocket.Conn, session *sessionCipher, connectionID, id, message string) error {
	reply, err := session.seal(map[string]any{
		"v": protocolVersion, "type": "error", "id": id, "error": message,
	})
	if err != nil {
		return err
	}
	return writeDirected(conn, connectionID, reply)
}

func (h *Host) taskCommand(ctx context.Context, deviceID string, scopes []account.RemoteCapability, command controllerCommand) map[string]any {
	response := map[string]any{"v": protocolVersion, "id": command.ID}
	if h.tasks == nil || !hasScope(scopes, account.RemoteTasks) {
		response["type"] = "error"
		response["error"] = "tasks access is unavailable"
		return response
	}
	var value any
	var err error
	switch command.Type {
	case "tasks.list":
		value, err = h.tasks.CloudTasks(ctx)
		response["type"] = "tasks.list"
		response["tasks"] = value
	case "tasks.get":
		value, err = h.tasks.CloudTask(ctx, command.TaskID)
		response["type"] = "tasks.get"
		response["snapshot"] = value
	case "tasks.send":
		err = h.tasks.CloudSubmit(ctx, command.TaskID, command.Text, deviceID)
		response["type"] = "tasks.sent"
	}
	if err != nil {
		response["type"] = "error"
		response["error"] = err.Error()
	}
	return response
}

func hasScope(scopes []account.RemoteCapability, want account.RemoteCapability) bool {
	return slices.Contains(scopes, want)
}

func writeDirected(conn *websocket.Conn, connectionID, payload string) error {
	wire, err := json.Marshal(map[string]string{"to": connectionID, "payload": payload})
	if err != nil {
		return err
	}
	if len(wire) > 64<<10 {
		return fmt.Errorf("remote cloud: reply exceeds relay limit")
	}
	return conn.WriteMessage(websocket.TextMessage, wire)
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
