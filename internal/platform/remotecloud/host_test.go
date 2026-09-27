package remotecloud

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestHandshakeAndPingCrossTheDirectedEncryptedChannel(t *testing.T) {
	connections := make(chan *websocket.Conn, 1)
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		connections <- conn
		<-done
	}))
	defer server.Close()
	controllerWire, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer controllerWire.Close()
	deviceWire := <-connections
	defer func() {
		deviceWire.Close()
		close(done)
	}()

	device, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	deviceID := strings.Repeat("d", 64)
	connectionID := strings.Repeat("c", 32)
	greeting, _ := json.Marshal(hello{
		Version: protocolVersion, Type: "hello",
		PublicKey: base64.RawURLEncoding.EncodeToString(controller.PublicKey().Bytes()),
		Salt:      base64.RawURLEncoding.EncodeToString(salt),
	})
	host := &Host{name: "Home Mac", version: "2.20.1"}
	sessions := make(map[string]*sessionCipher)
	if err := host.handle(deviceWire, &identity{DeviceID: deviceID}, device, sessions, mustJSON(t, gatewayMessage{
		Type: "controller_message", ConnectionID: connectionID, Payload: string(greeting),
	})); err != nil {
		t.Fatal(err)
	}
	controllerGreeting, _ := json.Marshal(hello{
		Version: protocolVersion, Type: "hello",
		PublicKey: base64.RawURLEncoding.EncodeToString(device.PublicKey().Bytes()),
		Salt:      base64.RawURLEncoding.EncodeToString(salt),
	})
	controllerCipher, err := newSessionCipher(controller, deviceID, string(controllerGreeting))
	if err != nil {
		t.Fatal(err)
	}
	ready := readDirected(t, controllerWire, controllerCipher)
	if ready["type"] != "ready" || ready["name"] != "Home Mac" {
		t.Fatalf("ready = %+v", ready)
	}

	ping, err := controllerCipher.seal(controllerCommand{Version: 1, Type: "ping", ID: "probe-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.handle(deviceWire, &identity{DeviceID: deviceID}, device, sessions, mustJSON(t, gatewayMessage{
		Type: "controller_message", ConnectionID: connectionID, Payload: ping,
	})); err != nil {
		t.Fatal(err)
	}
	pong := readDirected(t, controllerWire, controllerCipher)
	if pong["type"] != "pong" || pong["id"] != "probe-1" || pong["at"] == "" {
		t.Fatalf("pong = %+v", pong)
	}
}

func TestRegistrationPlatformUsesTheAccountContract(t *testing.T) {
	for input, want := range map[string]string{"darwin": "macos", "windows": "windows", "linux": "linux"} {
		if got := platformName(input); got != want {
			t.Errorf("platformName(%q) = %q, want %q", input, got, want)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func readDirected(t *testing.T, conn *websocket.Conn, channel *sessionCipher) map[string]any {
	t.Helper()
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var directed struct {
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(raw, &directed); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := channel.open(directed.Payload, &body); err != nil {
		t.Fatal(err)
	}
	return body
}
