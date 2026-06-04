package whatsapp

import (
	"context"
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

func TestProcessConversationMessagesPersistsReactionEvents(t *testing.T) {
	originalLog := log
	log = waLog.Noop
	defer func() { log = originalLog }()

	deviceID := "device-a@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	repo := &historyReactionRepoSpy{}

	ctx := ContextWithDevice(context.Background(), NewDeviceInstance(deviceID, nil, nil))
	syncType := waHistorySync.HistorySync_RECENT
	reactionTimestamp := uint64(time.Date(2026, time.May, 16, 8, 2, 0, 0, time.UTC).Unix())
	data := &waHistorySync.HistorySync{
		SyncType: &syncType,
		Conversations: []*waHistorySync.Conversation{
			{
				ID: proto.String(chatJID),
				Messages: []*waHistorySync.HistorySyncMsg{
					{
						Message: &waWeb.WebMessageInfo{
							Key: &waCommon.MessageKey{
								RemoteJID: proto.String(chatJID),
								FromMe:    proto.Bool(false),
								ID:        proto.String("reaction-event-1"),
							},
							Message: &waE2E.Message{
								ReactionMessage: &waE2E.ReactionMessage{
									Key: &waCommon.MessageKey{
										RemoteJID: proto.String(chatJID),
										FromMe:    proto.Bool(false),
										ID:        proto.String("msg-1"),
									},
									Text: proto.String("\U0001f44d"),
								},
							},
							MessageTimestamp: &reactionTimestamp,
						},
					},
				},
			},
		},
	}

	if err := processConversationMessages(ctx, data, repo, nil); err != nil {
		t.Fatalf("process conversation messages: %v", err)
	}

	if repo.createReactionCalls != 1 {
		t.Fatalf("expected history reaction event to be persisted once, got %d", repo.createReactionCalls)
	}
	if repo.lastReaction == nil {
		t.Fatal("expected reaction event to be passed to repository")
	}
	if got := repo.lastReaction.Message.GetReactionMessage().GetText(); got != "\U0001f44d" {
		t.Fatalf("expected thumbs-up reaction, got %q", got)
	}
	if got := repo.lastReaction.Message.GetReactionMessage().GetKey().GetID(); got != "msg-1" {
		t.Fatalf("expected target message id msg-1, got %q", got)
	}
}

func TestProcessConversationMessagesDoesNotRegressNewerChat(t *testing.T) {
	originalLog := log
	log = waLog.Noop
	defer func() { log = originalLog }()

	deviceID := "device-a@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"

	newerTime := time.Date(2026, time.May, 20, 9, 0, 0, 0, time.UTC)
	existing := &domainChatStorage.Chat{
		DeviceID:            deviceID,
		JID:                 chatJID,
		Name:                "Existing Name",
		LastMessageTime:     newerTime,
		EphemeralExpiration: 86400,
		Archived:            true,
	}
	repo := &historyChatMergeRepoSpy{existing: existing}

	ctx := ContextWithDevice(context.Background(), NewDeviceInstance(deviceID, nil, nil))
	syncType := waHistorySync.HistorySync_ON_DEMAND
	// Old message, older than the chat's current last_message_time.
	oldTimestamp := uint64(time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC).Unix())
	data := &waHistorySync.HistorySync{
		SyncType: &syncType,
		Conversations: []*waHistorySync.Conversation{
			{
				ID: proto.String(chatJID),
				Messages: []*waHistorySync.HistorySyncMsg{
					{
						Message: &waWeb.WebMessageInfo{
							Key: &waCommon.MessageKey{
								RemoteJID: proto.String(chatJID),
								FromMe:    proto.Bool(false),
								ID:        proto.String("old-msg-1"),
							},
							Message:          &waE2E.Message{Conversation: proto.String("an old message")},
							MessageTimestamp: &oldTimestamp,
						},
					},
				},
			},
		},
	}

	if err := processConversationMessages(ctx, data, repo, nil); err != nil {
		t.Fatalf("process conversation messages: %v", err)
	}

	if repo.stored == nil {
		t.Fatal("expected StoreChat to be called")
	}
	if !repo.stored.LastMessageTime.Equal(newerTime) {
		t.Fatalf("last_message_time regressed: got %s, want %s", repo.stored.LastMessageTime, newerTime)
	}
	if !repo.stored.Archived {
		t.Fatal("archived flag was reset by old-message sync")
	}
	if repo.stored.Name != "Existing Name" {
		t.Fatalf("chat name was overwritten: got %q", repo.stored.Name)
	}
	if repo.stored.EphemeralExpiration != 86400 {
		t.Fatalf("ephemeral timer was reset: got %d", repo.stored.EphemeralExpiration)
	}
	if repo.storedBatch != 1 {
		t.Fatalf("expected the old message to still be stored, got %d batches", repo.storedBatch)
	}
}

type historyChatMergeRepoSpy struct {
	domainChatStorage.IChatStorageRepository
	existing    *domainChatStorage.Chat
	stored      *domainChatStorage.Chat
	storedBatch int
}

func (r *historyChatMergeRepoSpy) GetChatNameWithPushName(jid types.JID, _ string, _ string, pushName string) string {
	if pushName != "" {
		return pushName
	}
	return jid.String()
}

func (r *historyChatMergeRepoSpy) GetChatByDevice(_, _ string) (*domainChatStorage.Chat, error) {
	return r.existing, nil
}

func (r *historyChatMergeRepoSpy) StoreChat(chat *domainChatStorage.Chat) error {
	r.stored = chat
	return nil
}

func (r *historyChatMergeRepoSpy) StoreMessagesBatch(_ []*domainChatStorage.Message) error {
	r.storedBatch++
	return nil
}

type historyReactionRepoSpy struct {
	domainChatStorage.IChatStorageRepository
	createReactionCalls int
	lastReaction        *events.Message
}

func (r *historyReactionRepoSpy) CreateReaction(_ context.Context, evt *events.Message) error {
	r.createReactionCalls++
	r.lastReaction = evt
	return nil
}

func (r *historyReactionRepoSpy) GetChatNameWithPushName(jid types.JID, _ string, _ string, pushName string) string {
	if pushName != "" {
		return pushName
	}
	return jid.String()
}
