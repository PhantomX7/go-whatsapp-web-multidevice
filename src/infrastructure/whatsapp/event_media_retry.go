package whatsapp

import (
	"context"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types/events"
)

// handleMediaRetry processes the phone's response to a media retry request
// (see chat usecase RepairMedia). The notification carries a fresh direct_path
// for a previously-stored message, letting us repair broken media downloads
// without re-pairing the device.
func handleMediaRetry(ctx context.Context, evt *events.MediaRetry, chatStorageRepo domainChatStorage.IChatStorageRepository) {
	if chatStorageRepo == nil || evt == nil {
		return
	}

	deviceID := ""
	if inst, ok := DeviceFromContext(ctx); ok && inst != nil {
		deviceID = inst.JID()
		if deviceID == "" {
			deviceID = inst.ID()
		}
	}
	if deviceID == "" {
		log.Warnf("Media retry for %s skipped: no device context", evt.MessageID)
		return
	}

	// The media key is required to decrypt the retry notification; fetch the
	// stored message (device-scoped) to obtain it.
	stored, err := chatStorageRepo.GetMessageByIDAndDevice(deviceID, evt.MessageID)
	if err != nil || stored == nil {
		log.Warnf("Media retry for %s skipped: message not found (%v)", evt.MessageID, err)
		return
	}
	if len(stored.MediaKey) == 0 {
		log.Warnf("Media retry for %s skipped: no stored media key", evt.MessageID)
		return
	}

	if evt.Error != nil {
		log.Warnf("Media retry for %s failed: %v", evt.MessageID, evt.Error)
		return
	}

	notification, err := whatsmeow.DecryptMediaRetryNotification(evt, stored.MediaKey)
	if err != nil {
		log.Warnf("Media retry for %s: failed to decrypt notification: %v", evt.MessageID, err)
		return
	}

	if notification.GetResult() != waMmsRetry.MediaRetryNotification_SUCCESS {
		log.Warnf("Media retry for %s not successful: %s", evt.MessageID, notification.GetResult().String())
		return
	}

	directPath := notification.GetDirectPath()
	if directPath == "" {
		log.Warnf("Media retry for %s succeeded but returned no direct path", evt.MessageID)
		return
	}

	if err := chatStorageRepo.UpdateMessageMediaReference(deviceID, evt.MessageID, stored.ChatJID, directPath); err != nil {
		log.Warnf("Media retry for %s: failed to update direct_path: %v", evt.MessageID, err)
		return
	}

	log.Infof("Media retry: refreshed direct_path for message %s in chat %s", evt.MessageID, stored.ChatJID)
}
