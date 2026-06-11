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

func TestMediaDirectPathRoundTrips(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	deviceID := "device-a@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	ts := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)

	// StoreChat first to satisfy listing; then store a media message via StoreMessage.
	if err := repo.StoreChat(&domainChatStorage.Chat{DeviceID: deviceID, JID: chatJID, Name: chatJID, LastMessageTime: ts}); err != nil {
		t.Fatalf("store chat: %v", err)
	}
	if err := repo.StoreMessage(&domainChatStorage.Message{
		ID:         "img-1",
		ChatJID:    chatJID,
		DeviceID:   deviceID,
		Sender:     "628999999999@s.whatsapp.net",
		Content:    "",
		Timestamp:  ts,
		MediaType:  "image",
		URL:        "https://mmg.whatsapp.net/v/t62.7118-24/abc.enc",
		DirectPath: "/v/t62.7118-24/abc.enc?ccb=11-4",
	}); err != nil {
		t.Fatalf("store message: %v", err)
	}

	// Via single-row lookup (used by the download endpoint).
	got, err := repo.GetMessageByID("img-1")
	if err != nil || got == nil {
		t.Fatalf("get message by id: %v", err)
	}
	if got.DirectPath != "/v/t62.7118-24/abc.enc?ccb=11-4" {
		t.Fatalf("direct_path not persisted via StoreMessage: got %q", got.DirectPath)
	}

	// Via batch insert/update path (used by history sync).
	if err := repo.StoreMessagesBatch([]*domainChatStorage.Message{{
		ID:         "img-2",
		ChatJID:    chatJID,
		DeviceID:   deviceID,
		Sender:     "628999999999@s.whatsapp.net",
		Timestamp:  ts.Add(time.Minute),
		MediaType:  "video",
		DirectPath: "/v/t62.7118-24/def.enc?ccb=11-4",
	}}); err != nil {
		t.Fatalf("store batch: %v", err)
	}
	got2, err := repo.GetMessageByID("img-2")
	if err != nil || got2 == nil {
		t.Fatalf("get batch message: %v", err)
	}
	if got2.DirectPath != "/v/t62.7118-24/def.enc?ccb=11-4" {
		t.Fatalf("direct_path not persisted via StoreMessagesBatch: got %q", got2.DirectPath)
	}
}

func TestGetMediaMessagesForRepairAndUpdateReference(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	deviceID := "device-a@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	ts := time.Date(2026, time.May, 16, 8, 0, 0, 0, time.UTC)

	if err := repo.StoreChat(&domainChatStorage.Chat{DeviceID: deviceID, JID: chatJID, Name: chatJID, LastMessageTime: ts}); err != nil {
		t.Fatalf("store chat: %v", err)
	}

	store := func(id, mediaType, directPath string, key []byte, offset time.Duration) {
		if err := repo.StoreMessage(&domainChatStorage.Message{
			ID: id, ChatJID: chatJID, DeviceID: deviceID, Sender: "628999999999@s.whatsapp.net",
			Timestamp: ts.Add(offset), MediaType: mediaType, DirectPath: directPath, MediaKey: key,
		}); err != nil {
			t.Fatalf("store message %s: %v", id, err)
		}
	}

	// Candidate: media + key + no direct_path.
	store("broken-1", "image", "", []byte("key-1"), time.Minute)
	// Not a candidate: already has direct_path.
	store("ok-1", "image", "/v/already.enc", []byte("key-2"), 2*time.Minute)
	// Not a candidate: no media key (can't decrypt a retry).
	store("nokey-1", "video", "", nil, 3*time.Minute)
	// Not a candidate: plain text (no media).
	if err := repo.StoreMessage(&domainChatStorage.Message{
		ID: "text-1", ChatJID: chatJID, DeviceID: deviceID, Sender: "628999999999@s.whatsapp.net",
		Timestamp: ts.Add(4 * time.Minute), Content: "hello",
	}); err != nil {
		t.Fatalf("store text: %v", err)
	}

	candidates, err := repo.GetMediaMessagesForRepair(deviceID, chatJID, 50)
	if err != nil {
		t.Fatalf("get repair candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != "broken-1" {
		ids := make([]string, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		t.Fatalf("expected only broken-1 as candidate, got %v", ids)
	}

	// After updating the reference, it's no longer a candidate (idempotent).
	if err := repo.UpdateMessageMediaReference(deviceID, "broken-1", chatJID, "/v/refreshed.enc"); err != nil {
		t.Fatalf("update media reference: %v", err)
	}
	got, err := repo.GetMessageByID("broken-1")
	if err != nil || got == nil {
		t.Fatalf("get broken-1: %v", err)
	}
	if got.DirectPath != "/v/refreshed.enc" {
		t.Fatalf("direct_path not updated: got %q", got.DirectPath)
	}
	again, err := repo.GetMediaMessagesForRepair(deviceID, chatJID, 50)
	if err != nil {
		t.Fatalf("get repair candidates (2nd): %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected no candidates after repair, got %d", len(again))
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

// TestCountMessagesMatchesFilteredResults pins the pagination-total fix: the
// count must apply the exact same filter as GetMessages (date range, content
// search, device isolation) and compose them — otherwise the UI reports the
// whole chat's size as the "match" count (e.g. "139 matches" for a handful of
// results).
func TestCountMessagesMatchesFilteredResults(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	deviceID := "device-a@s.whatsapp.net"
	otherDeviceID := "device-b@s.whatsapp.net"
	chatJID := "628123456789@s.whatsapp.net"
	day1 := time.Date(2026, time.June, 9, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC)

	seedChatMessage(t, repo, deviceID, chatJID, "msg-1", "Pesan pagi hari", day1.Add(9*time.Hour))
	seedChatMessage(t, repo, deviceID, chatJID, "msg-2", "bosspack ke tangerang", day1.Add(14*time.Hour))
	seedChatMessage(t, repo, deviceID, chatJID, "msg-3", "ok pak terimakasih", day2.Add(10*time.Hour))
	// Another device's message in the same chat/day with the search term must
	// never be counted for deviceID.
	seedChatMessage(t, repo, otherDeviceID, chatJID, "msg-other", "bosspack lain", day1.Add(12*time.Hour))

	day1End := day1.Add(24*time.Hour - time.Second)
	searchTerm := "BOSSPACK" // upper-case to assert case-insensitive matching

	cases := []struct {
		name   string
		filter *domainChatStorage.MessageFilter
		want   int64
	}{
		{
			name:   "no content/time filter counts the device's whole chat",
			filter: &domainChatStorage.MessageFilter{DeviceID: deviceID, ChatJID: chatJID},
			want:   3,
		},
		{
			name:   "date range counts only that day",
			filter: &domainChatStorage.MessageFilter{DeviceID: deviceID, ChatJID: chatJID, StartTime: &day1, EndTime: &day1End},
			want:   2,
		},
		{
			name:   "search counts only matching content, device-isolated",
			filter: &domainChatStorage.MessageFilter{DeviceID: deviceID, ChatJID: chatJID, Search: searchTerm},
			want:   1,
		},
		{
			name:   "search composes with date range",
			filter: &domainChatStorage.MessageFilter{DeviceID: deviceID, ChatJID: chatJID, Search: searchTerm, StartTime: &day2, EndTime: &day2},
			want:   0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			count, err := repo.CountMessages(tc.filter)
			if err != nil {
				t.Fatalf("count messages: %v", err)
			}
			if count != tc.want {
				t.Fatalf("CountMessages = %d, want %d", count, tc.want)
			}
			// The count must equal the number of rows a paged GetMessages call
			// returns for the same filter — that invariant is the whole point.
			messages, err := repo.GetMessages(tc.filter)
			if err != nil {
				t.Fatalf("get messages: %v", err)
			}
			if int64(len(messages)) != tc.want {
				t.Fatalf("GetMessages returned %d rows, want %d (count drift from total)", len(messages), tc.want)
			}
		})
	}

	// device_id is required for data isolation, mirroring GetMessages.
	if _, err := repo.CountMessages(&domainChatStorage.MessageFilter{ChatJID: chatJID}); err == nil {
		t.Fatal("expected error when device_id is missing")
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
