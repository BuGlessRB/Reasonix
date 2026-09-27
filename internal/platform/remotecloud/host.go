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
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"reasonix/internal/platform/account"
)

const DefaultRelayURL = "wss://remote.reasonix.io"

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

	mu    sync.RWMutex
	state hostState
}

func New(client *account.Client, dialer *websocket.Dialer, relayURL, version string) *Host {
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
	return &Host{
		client: client, dialer: dialer, relayURL: strings.TrimRight(relayURL, "/"),
		version: version, name: name, token: account.Token,
	}
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
		if keyErr == nil {
			return saved, private, nil
		}
	}
	private, public, err := generatePrivateKey()
	if err != nil {
		return nil, nil, err
	}
	registered, err := h.client.RegisterRemoteDevice(ctx, token, account.RemoteDeviceRegistration{
		Name: h.name, Platform: platformName(runtime.GOOS), PublicKey: public,
		Capabilities: []account.RemoteCapability{account.RemoteTasks, account.RemoteLogs, account.RemoteFiles},
	})
	if err != nil {
		return nil, nil, err
	}
	saved = &identity{
		OwnerID: user.ID, DeviceID: registered.Device.ID,
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
			if err := h.handle(conn, saved, private, sessions, payload); err != nil {
				continue
			}
		}
	}
}

func (h *Host) handle(
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
	if command.Version != protocolVersion || command.Type != "ping" || command.ID == "" {
		return errors.New("remote cloud: unsupported controller command")
	}
	pong, err := session.seal(map[string]any{
		"v": protocolVersion, "type": "pong", "id": command.ID,
		"at": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	return writeDirected(conn, message.ConnectionID, pong)
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
