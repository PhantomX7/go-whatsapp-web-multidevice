package chatstorage

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type IChatStorageRepository interface {
	// Chat operations
	CreateMessage(ctx context.Context, evt *events.Message) error
	CreateReaction(ctx context.Context, evt *events.Message) error
	// CreateIncomingCallRecord persists an incoming call as a synthetic message (media_type "call") for chat history.
	CreateIncomingCallRecord(ctx context.Context, evt *events.CallOffer, autoRejected bool) error
	StoreChat(chat *Chat) error
	GetChat(jid string) (*Chat, error)
	GetChatByDevice(deviceID, jid string) (*Chat, error)
	GetChats(filter *ChatFilter) ([]*Chat, error)
	DeleteChat(jid string) error
	DeleteChatByDevice(deviceID, jid string) error

	// Message operations
	StoreMessage(message *Message) error
	StoreMessageEdit(edit *MessageEdit) error
	StoreMessagesBatch(messages []*Message) error
	GetMessageByID(id string) (*Message, error)                    // New method for efficient ID-only search
	GetMessageByIDAndDevice(deviceID, id string) (*Message, error) // Device-scoped ID lookup for device-isolated flows
	GetOldestMessage(deviceID, chatJID string) (*Message, error)   // Oldest stored message for a chat (anchor for on-demand history sync)
	GetMediaMessagesForRepair(deviceID, chatJID string, limit int) ([]*Message, error) // Media messages missing a direct_path (candidates for media retry)
	UpdateMessageMediaReference(deviceID, id, chatJID, directPath string) error         // Refresh a message's direct_path after a media retry
	GetMessageEdits(originalMessageID, deviceID string) ([]*MessageEdit, error)
	GetMessages(filter *MessageFilter) ([]*Message, error)
	CountMessages(filter *MessageFilter) (int64, error)                                  // Count of messages matching the same filter as GetMessages (for accurate pagination totals)
	SearchMessages(deviceID, chatJID, searchText string, limit int) ([]*Message, error) // Database-level search with device isolation
	DeleteMessage(id, chatJID string) error
	DeleteMessageByDevice(deviceID, id, chatJID string) error
	StoreSentMessageWithContext(ctx context.Context, messageID string, senderJID string, recipientJID string, content string, timestamp time.Time, msg *waE2E.Message) error

	// Chatwoot correlation operations
	UpsertChatwootMessageLink(link *ChatwootMessageLink) error
	GetChatwootMessageLinkByWhatsAppID(deviceID, waMessageID string) (*ChatwootMessageLink, error)
	GetChatwootMessageLinkByChatwootID(deviceID string, chatwootMessageID int) (*ChatwootMessageLink, error)
	GetLatestChatwootMessageLinkByConversation(conversationID int) (*ChatwootMessageLink, error)
	GetLatestUnreadChatwootMessageLinkByChat(deviceID, waChatJID string) (*ChatwootMessageLink, error)
	EnqueueChatwootForwardEvent(event *ChatwootForwardEvent) error
	ListDueChatwootForwardEvents(now time.Time, limit int) ([]*ChatwootForwardEvent, error)
	MarkChatwootForwardEventFailed(id int64, lastError string, nextAttemptAt time.Time) error
	MarkChatwootForwardEventDone(id int64) error

	// Statistics
	GetChatMessageCount(chatJID string) (int64, error)
	GetChatMessageCountByDevice(deviceID, chatJID string) (int64, error)
	GetTotalMessageCount() (int64, error)
	GetTotalChatCount() (int64, error)
	GetFilteredChatCount(filter *ChatFilter) (int64, error)
	GetChatNameWithPushName(jid types.JID, chatJID string, senderUser string, pushName string) string
	GetChatNameWithPushNameByDevice(deviceID string, jid types.JID, chatJID string, senderUser string, pushName string) string
	GetStorageStatistics() (chatCount int64, messageCount int64, err error)

	// Maintenance operations
	RepairChatLastMessageTimes() (int64, error) // Recompute last_message_time from newest message (fixes history-sync regressions)

	// Cleanup operations
	TruncateAllChats() error
	TruncateAllDataWithLogging(logPrefix string) error
	DeleteDeviceData(deviceID string) error

	// Device registry operations
	SaveDeviceRecord(record *DeviceRecord) error
	ListDeviceRecords() ([]*DeviceRecord, error)
	GetDeviceRecord(deviceID string) (*DeviceRecord, error)
	DeleteDeviceRecord(deviceID string) error

	// Schema operations
	InitializeSchema() error
}
