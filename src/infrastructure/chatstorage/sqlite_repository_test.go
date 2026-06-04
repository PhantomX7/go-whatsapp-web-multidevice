package chatstorage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/sqlite"
)

func newTestSQLiteRepository(t *testing.T) *SQLiteRepository {
	t.Helper()

	db, err := sql.Open(sqlite.DriverName, filepath.Join(t.TempDir(), "chatstorage.db"))
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := &SQLiteRepository{db: db}
	if err := repo.InitializeSchema(); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo
}

func TestSQLiteRepositoryInitializesMessageReactionsSchema(t *testing.T) {
	repo := newTestSQLiteRepository(t)

	var tableName string
	err := repo.db.QueryRow(`
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = 'message_reactions'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("expected message_reactions table to exist: %v", err)
	}
	if tableName != "message_reactions" {
		t.Fatalf("expected message_reactions table, got %q", tableName)
	}
}

func TestSQLiteRepositoryStoresUpdatesRemovesAndHydratesReactions(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	deviceID := "device-a@s.whatsapp.net"
	otherDeviceID := "device-b@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)

	seedChatMessage(t, repo, deviceID, chatJID, "msg-1", "hello reaction", now)
	seedChatMessage(t, repo, otherDeviceID, chatJID, "msg-1", "hello other device", now)

	if err := repo.StoreReaction(&domainChatStorage.Reaction{
		MessageID:  "msg-1",
		ChatJID:    chatJID,
		DeviceID:   deviceID,
		ReactorJID: "628111111111@s.whatsapp.net",
		Emoji:      "\U0001f44d",
		IsFromMe:   false,
		Timestamp:  now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("store reaction: %v", err)
	}
	if err := repo.StoreReaction(&domainChatStorage.Reaction{
		MessageID:  "msg-1",
		ChatJID:    chatJID,
		DeviceID:   otherDeviceID,
		ReactorJID: "628222222222@s.whatsapp.net",
		Emoji:      "\U0001f525",
		IsFromMe:   false,
		Timestamp:  now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("store other device reaction: %v", err)
	}

	messages := getMessagesForTest(t, repo, deviceID, chatJID)
	if got := len(messages[0].Reactions); got != 1 {
		t.Fatalf("expected one device-scoped reaction, got %d", got)
	}
	if got := messages[0].Reactions[0].Emoji; got != "\U0001f44d" {
		t.Fatalf("expected hydrated thumbs-up reaction, got %q", got)
	}

	if err := repo.StoreReaction(&domainChatStorage.Reaction{
		MessageID:  "msg-1",
		ChatJID:    chatJID,
		DeviceID:   deviceID,
		ReactorJID: "628111111111@s.whatsapp.net",
		Emoji:      "\U0001f525",
		IsFromMe:   false,
		Timestamp:  now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("update reaction: %v", err)
	}

	messages = getMessagesForTest(t, repo, deviceID, chatJID)
	if got := len(messages[0].Reactions); got != 1 {
		t.Fatalf("expected one updated reaction, got %d", got)
	}
	if got := messages[0].Reactions[0].Emoji; got != "\U0001f525" {
		t.Fatalf("expected updated fire reaction, got %q", got)
	}

	searchResults, err := repo.SearchMessages(deviceID, chatJID, "reaction", 10)
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if got := len(searchResults); got != 1 {
		t.Fatalf("expected one search result, got %d", got)
	}
	if got := len(searchResults[0].Reactions); got != 1 {
		t.Fatalf("expected search result to hydrate reactions, got %d", got)
	}

	if err := repo.StoreReaction(&domainChatStorage.Reaction{
		MessageID:  "msg-1",
		ChatJID:    chatJID,
		DeviceID:   deviceID,
		ReactorJID: "628111111111@s.whatsapp.net",
		Emoji:      "",
		Timestamp:  now.Add(4 * time.Minute),
	}); err != nil {
		t.Fatalf("remove reaction: %v", err)
	}

	messages = getMessagesForTest(t, repo, deviceID, chatJID)
	if got := len(messages[0].Reactions); got != 0 {
		t.Fatalf("expected reaction removal to clear reactions, got %d", got)
	}
}

func TestSQLiteRepositoryDeletesReactionsWithMessagesAndDevices(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	deviceID := "device-a@s.whatsapp.net"
	otherDeviceID := "device-b@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	now := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)

	seedChatMessage(t, repo, deviceID, chatJID, "msg-1", "hello", now)
	seedReaction(t, repo, deviceID, chatJID, "msg-1", "628111111111@s.whatsapp.net")
	seedChatMessage(t, repo, otherDeviceID, chatJID, "msg-1", "hello", now)
	seedReaction(t, repo, otherDeviceID, chatJID, "msg-1", "628222222222@s.whatsapp.net")

	if err := repo.DeleteMessageByDevice(deviceID, "msg-1", chatJID); err != nil {
		t.Fatalf("delete message by device: %v", err)
	}
	if got := countMessageReactions(t, repo); got != 1 {
		t.Fatalf("expected only other device reaction to remain, got %d", got)
	}

	if err := repo.DeleteDeviceData(otherDeviceID); err != nil {
		t.Fatalf("delete device data: %v", err)
	}
	if got := countMessageReactions(t, repo); got != 0 {
		t.Fatalf("expected device cleanup to delete reactions, got %d", got)
	}
}

func TestGetOldestMessageReturnsEarliestDeviceScopedMessage(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	deviceID := "device-a@s.whatsapp.net"
	otherDeviceID := "device-b@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	base := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)

	// Seed out of chronological order to ensure ordering, not insertion order, decides the anchor.
	seedChatMessage(t, repo, deviceID, chatJID, "msg-newer", "newer", base.Add(2*time.Hour))
	seedChatMessage(t, repo, deviceID, chatJID, "msg-oldest", "oldest", base)
	seedChatMessage(t, repo, deviceID, chatJID, "msg-mid", "mid", base.Add(time.Hour))
	// A message belonging to another device must never be selected.
	seedChatMessage(t, repo, otherDeviceID, chatJID, "msg-other-older", "other device older", base.Add(-time.Hour))

	oldest, err := repo.GetOldestMessage(deviceID, chatJID)
	if err != nil {
		t.Fatalf("get oldest message: %v", err)
	}
	if oldest == nil {
		t.Fatal("expected an oldest message, got nil")
	}
	if oldest.ID != "msg-oldest" {
		t.Fatalf("expected oldest message msg-oldest, got %q", oldest.ID)
	}

	// Empty chat returns (nil, nil) so callers can detect the no-anchor case.
	empty, err := repo.GetOldestMessage(deviceID, "000000@s.whatsapp.net")
	if err != nil {
		t.Fatalf("get oldest message for empty chat: %v", err)
	}
	if empty != nil {
		t.Fatalf("expected nil for chat without messages, got %q", empty.ID)
	}

	// device_id is required for data isolation.
	if _, err := repo.GetOldestMessage("", chatJID); err == nil {
		t.Fatal("expected error when device_id is missing")
	}
}

func TestRepairChatLastMessageTimesFixesRegressedChats(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	deviceID := "device-a@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	newest := time.Date(2026, time.May, 20, 9, 0, 0, 0, time.UTC)
	older := time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC)

	// A message exists at the newer time (seedChatMessage also sets the chat time).
	seedChatMessage(t, repo, deviceID, chatJID, "msg-new", "newest", newest)
	// Simulate the bug: the chat row was regressed to an older time (and archived).
	if err := repo.StoreChat(&domainChatStorage.Chat{
		DeviceID:        deviceID,
		JID:             chatJID,
		Name:            "Chat A",
		LastMessageTime: older,
		Archived:        true,
	}); err != nil {
		t.Fatalf("store regressed chat: %v", err)
	}

	repaired, err := repo.RepairChatLastMessageTimes()
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if repaired != 1 {
		t.Fatalf("expected 1 chat repaired, got %d", repaired)
	}

	chat, err := repo.GetChatByDevice(deviceID, chatJID)
	if err != nil || chat == nil {
		t.Fatalf("get chat: %v", err)
	}
	if !chat.LastMessageTime.Equal(newest) {
		t.Fatalf("last_message_time not repaired: got %s, want %s", chat.LastMessageTime, newest)
	}
	// Repair must not touch unrelated fields.
	if !chat.Archived {
		t.Fatal("repair should not change archived flag")
	}
	if chat.Name != "Chat A" {
		t.Fatalf("repair should not change name: got %q", chat.Name)
	}

	// Idempotent: a second run on a now-healthy db repairs nothing.
	repaired, err = repo.RepairChatLastMessageTimes()
	if err != nil {
		t.Fatalf("repair (2nd): %v", err)
	}
	if repaired != 0 {
		t.Fatalf("expected 0 chats repaired on healthy db, got %d", repaired)
	}
}

func TestStoreSentMessageWithContextRequiresDeviceInContext(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	deviceID := "6289605618749@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	now := time.Date(2026, time.May, 22, 10, 0, 0, 0, time.UTC)

	err := repo.StoreSentMessageWithContext(
		context.Background(),
		"msg-sent-1",
		deviceID,
		chatJID,
		"hello from api",
		now,
		nil,
	)
	if err == nil {
		t.Fatal("expected error when storing sent message without device context")
	}
	if !errors.Is(err, domainChatStorage.ErrMissingDeviceContext) {
		t.Fatalf("expected missing device context error, got %v", err)
	}
}

func seedChatMessage(t *testing.T, repo *SQLiteRepository, deviceID, chatJID, messageID, content string, timestamp time.Time) {
	t.Helper()
	if err := repo.StoreChat(&domainChatStorage.Chat{
		DeviceID:        deviceID,
		JID:             chatJID,
		Name:            chatJID,
		LastMessageTime: timestamp,
	}); err != nil {
		t.Fatalf("store chat: %v", err)
	}
	if err := repo.StoreMessage(&domainChatStorage.Message{
		ID:        messageID,
		ChatJID:   chatJID,
		DeviceID:  deviceID,
		Sender:    "628999999999@s.whatsapp.net",
		Content:   content,
		Timestamp: timestamp,
	}); err != nil {
		t.Fatalf("store message: %v", err)
	}
}

func seedReaction(t *testing.T, repo *SQLiteRepository, deviceID, chatJID, messageID, reactorJID string) {
	t.Helper()
	if err := repo.StoreReaction(&domainChatStorage.Reaction{
		MessageID:  messageID,
		ChatJID:    chatJID,
		DeviceID:   deviceID,
		ReactorJID: reactorJID,
		Emoji:      "\U0001f44d",
		Timestamp:  time.Date(2026, time.May, 16, 8, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("store reaction: %v", err)
	}
}

func getMessagesForTest(t *testing.T, repo *SQLiteRepository, deviceID, chatJID string) []*domainChatStorage.Message {
	t.Helper()
	messages, err := repo.GetMessages(&domainChatStorage.MessageFilter{
		DeviceID: deviceID,
		ChatJID:  chatJID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one message, got %d", len(messages))
	}
	return messages
}

func countMessageReactions(t *testing.T, repo *SQLiteRepository) int {
	t.Helper()
	var count int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM message_reactions`).Scan(&count); err != nil {
		t.Fatalf("count message reactions: %v", err)
	}
	return count
}
