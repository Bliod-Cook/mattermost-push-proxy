package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/stretchr/testify/require"
)

func TestNtfyInitialize_DefaultServerURL(t *testing.T) {
	logger, err := mlog.NewLogger()
	require.NoError(t, err)

	srv := NewNtfyNotificationServer(NtfyPushSettings{Type: PushNotifyNtfy}, logger, nil, 10)
	require.NoError(t, srv.Initialize())
	require.Equal(t, "https://ntfy.sh", srv.NtfyPushSettings.ServerURL)
}

func TestNtfySendNotification_Success(t *testing.T) {
	logger, err := mlog.NewLogger()
	require.NoError(t, err)

	var gotPath string
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	srv := NewNtfyNotificationServer(NtfyPushSettings{
		Type:        PushNotifyNtfy,
		ServerURL:   ts.URL,
		TopicPrefix: "mm",
	}, logger, nil, 10)
	require.NoError(t, srv.Initialize())

	resp := srv.SendNotification(&PushNotification{
		ID:       "msg-1",
		Type:     PushTypeMessage,
		ServerID: "server-1",
		DeviceID: "device-topic",
		Message:  "hello ntfy",
	})

	require.Equal(t, PUSH_STATUS_OK, resp[PUSH_STATUS])
	require.Equal(t, "/mm-device-topic", gotPath)
	require.Equal(t, "hello ntfy", gotBody)
}

func TestNtfySendNotification_Failure(t *testing.T) {
	logger, err := mlog.NewLogger()
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer ts.Close()

	srv := NewNtfyNotificationServer(NtfyPushSettings{Type: PushNotifyNtfy, ServerURL: ts.URL}, logger, nil, 10)
	require.NoError(t, srv.Initialize())

	resp := srv.SendNotification(&PushNotification{Type: PushTypeMessage, ServerID: "server-1", DeviceID: "device-topic"})
	require.Equal(t, PUSH_STATUS_FAIL, resp[PUSH_STATUS])
}
