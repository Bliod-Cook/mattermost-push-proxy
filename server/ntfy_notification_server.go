package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

type NtfyNotificationServer struct {
	client           *http.Client
	logger           *mlog.Logger
	metrics          *metrics
	NtfyPushSettings NtfyPushSettings
}

func NewNtfyNotificationServer(settings NtfyPushSettings, logger *mlog.Logger, metrics *metrics, sendTimeoutSecs int) *NtfyNotificationServer {
	return &NtfyNotificationServer{
		NtfyPushSettings: settings,
		logger:           logger,
		metrics:          metrics,
		client: &http.Client{
			Timeout: time.Duration(sendTimeoutSecs) * time.Second,
		},
	}
}

func (me *NtfyNotificationServer) Initialize() error {
	me.logger.Info("Initializing ntfy notification server", mlog.String("type", me.NtfyPushSettings.Type))

	if me.NtfyPushSettings.ServerURL == "" {
		me.NtfyPushSettings.ServerURL = "https://ntfy.sh"
	}

	if _, err := url.ParseRequestURI(me.NtfyPushSettings.ServerURL); err != nil {
		return fmt.Errorf("invalid ntfy server url: %w", err)
	}

	return nil
}

func (me *NtfyNotificationServer) SendNotification(msg *PushNotification) PushResponse {
	pushType := msg.Type
	if me.metrics != nil {
		me.metrics.incrementNotificationTotal(PushNotifyNtfy, pushType)
	}

	topic := strings.TrimSpace(msg.DeviceID)
	if me.NtfyPushSettings.TopicPrefix != "" {
		topic = fmt.Sprintf("%s-%s", me.NtfyPushSettings.TopicPrefix, topic)
	}
	if topic == "" {
		errMsg := "missing ntfy topic derived from device id"
		if me.metrics != nil {
			me.metrics.incrementFailure(PushNotifyNtfy, pushType, "missing_topic")
		}
		return NewErrorPushResponse(errMsg)
	}

	messageText := strings.TrimSpace(msg.Message)
	if messageText == "" {
		messageText = fmt.Sprintf("Mattermost push: %s", pushType)
	}

	endpoint := strings.TrimRight(me.NtfyPushSettings.ServerURL, "/") + "/" + url.PathEscape(topic)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(messageText))
	if err != nil {
		if me.metrics != nil {
			me.metrics.incrementFailure(PushNotifyNtfy, pushType, "build_request")
		}
		return NewErrorPushResponse(err.Error())
	}

	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("X-Title", fmt.Sprintf("Mattermost %s", pushType))
	req.Header.Set("X-Server-ID", msg.ServerID)
	req.Header.Set("X-Message-ID", msg.ID)
	req.Header.Set("X-Tags", strings.Join(me.NtfyPushSettings.Tags, ","))
	if me.NtfyPushSettings.Priority != "" {
		req.Header.Set("X-Priority", me.NtfyPushSettings.Priority)
	}
	if me.NtfyPushSettings.AuthorizationToken != "" {
		req.Header.Set("Authorization", "Bearer "+me.NtfyPushSettings.AuthorizationToken)
	}
	if me.NtfyPushSettings.Username != "" {
		req.SetBasicAuth(me.NtfyPushSettings.Username, me.NtfyPushSettings.Password)
	}

	start := time.Now()
	resp, err := me.client.Do(req)
	if me.metrics != nil {
		me.metrics.observerNotificationResponse(PushNotifyNtfy, time.Since(start).Seconds())
	}
	if err != nil {
		if me.metrics != nil {
			me.metrics.incrementFailure(PushNotifyNtfy, pushType, "request_error")
		}
		me.logger.Error("Failed to send ntfy push", mlog.Err(err), mlog.String("type", me.NtfyPushSettings.Type), mlog.String("did", msg.DeviceID))
		return NewErrorPushResponse(err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		errMsg := fmt.Sprintf("ntfy push failed with status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		if me.metrics != nil {
			me.metrics.incrementFailure(PushNotifyNtfy, pushType, fmt.Sprintf("status_%d", resp.StatusCode))
		}
		return NewErrorPushResponse(errMsg)
	}

	if me.metrics != nil {
		if msg.AckID != "" {
			me.metrics.incrementSuccessWithAck(PushNotifyNtfy, pushType)
		} else {
			me.metrics.incrementSuccess(PushNotifyNtfy, pushType)
		}
	}

	return NewOkPushResponse()
}
