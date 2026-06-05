package usecase

import (
	"context"
	"fmt"
	"time"

	domainChat "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chat"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/validations"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

type serviceChat struct {
	chatStorageRepo domainChatStorage.IChatStorageRepository
}

func NewChatService(chatStorageRepo domainChatStorage.IChatStorageRepository) domainChat.IChatUsecase {
	return &serviceChat{
		chatStorageRepo: chatStorageRepo,
	}
}

func (service serviceChat) ListChats(ctx context.Context, request domainChat.ListChatsRequest) (response domainChat.ListChatsResponse, err error) {
	if err = validations.ValidateListChats(ctx, &request); err != nil {
		return response, err
	}

	// Create filter from request
	filter := &domainChatStorage.ChatFilter{
		DeviceID:   deviceIDFromContext(ctx),
		Limit:      request.Limit,
		Offset:     request.Offset,
		SearchName: request.Search,
		HasMedia:   request.HasMedia,
		IsArchived: request.Archived,
	}

	// Get chats from storage
	chats, err := service.chatStorageRepo.GetChats(filter)
	if err != nil {
		logrus.WithError(err).Error("Failed to get chats from storage")
		return response, err
	}

	// Get total count for pagination (with same filters for accuracy)
	totalCount, err := service.chatStorageRepo.GetFilteredChatCount(filter)
	if err != nil {
		logrus.WithError(err).Error("Failed to get total chat count")
		// Continue with partial data
		totalCount = 0
	}

	// Convert entities to domain objects
	chatInfos := make([]domainChat.ChatInfo, 0, len(chats))
	for _, chat := range chats {
		chatInfo := domainChat.ChatInfo{
			JID:                 chat.JID,
			Name:                chat.Name,
			LastMessageTime:     chat.LastMessageTime.Format(time.RFC3339),
			EphemeralExpiration: chat.EphemeralExpiration,
			CreatedAt:           chat.CreatedAt.Format(time.RFC3339),
			UpdatedAt:           chat.UpdatedAt.Format(time.RFC3339),
			Archived:            chat.Archived,
		}
		chatInfos = append(chatInfos, chatInfo)
	}

	// Create pagination response
	pagination := domainChat.PaginationResponse{
		Limit:  request.Limit,
		Offset: request.Offset,
		Total:  int(totalCount),
	}

	response.Data = chatInfos
	response.Pagination = pagination

	logrus.WithFields(logrus.Fields{
		"total_chats": len(chatInfos),
		"limit":       request.Limit,
		"offset":      request.Offset,
	}).Info("Listed chats successfully")

	return response, nil
}

func (service serviceChat) GetChatMessages(ctx context.Context, request domainChat.GetChatMessagesRequest) (response domainChat.GetChatMessagesResponse, err error) {
	if err = validations.ValidateGetChatMessages(ctx, &request); err != nil {
		return response, err
	}

	deviceID := deviceIDFromContext(ctx)
	if deviceID == "" {
		return response, fmt.Errorf("device identification required")
	}

	chat, err := service.chatStorageRepo.GetChatByDevice(deviceID, request.ChatJID)
	if err != nil {
		logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to get chat info")
		return response, err
	}
	if chat == nil {
		return response, fmt.Errorf("chat with JID %s not found", request.ChatJID)
	}

	// Create message filter from request
	filter := &domainChatStorage.MessageFilter{
		ChatJID:   request.ChatJID,
		Limit:     request.Limit,
		Offset:    request.Offset,
		MediaOnly: request.MediaOnly,
		IsFromMe:  request.IsFromMe,
	}

	// Parse time filters if provided
	if request.StartTime != nil && *request.StartTime != "" {
		startTime, err := time.Parse(time.RFC3339, *request.StartTime)
		if err != nil {
			return response, fmt.Errorf("invalid start_time format: %v", err)
		}
		filter.StartTime = &startTime
	}

	if request.EndTime != nil && *request.EndTime != "" {
		endTime, err := time.Parse(time.RFC3339, *request.EndTime)
		if err != nil {
			return response, fmt.Errorf("invalid end_time format: %v", err)
		}
		filter.EndTime = &endTime
	}

	// Get messages from storage
	var messages []*domainChatStorage.Message
	if request.Search != "" {
		// Use search functionality if search query is provided
		messages, err = service.chatStorageRepo.SearchMessages(deviceID, request.ChatJID, request.Search, request.Limit)
		if err != nil {
			logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to search messages")
			return response, err
		}
	} else {
		// Use regular filter with device_id for data isolation
		filter.DeviceID = deviceID
		messages, err = service.chatStorageRepo.GetMessages(filter)
		if err != nil {
			logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to get messages")
			return response, err
		}
	}

	// Get total message count for pagination
	totalCount, err := service.chatStorageRepo.GetChatMessageCount(request.ChatJID)
	if err != nil {
		logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to get message count")
		// Continue with partial data
		totalCount = 0
	}

	// Convert entities to domain objects
	messageInfos := make([]domainChat.MessageInfo, 0, len(messages))
	for _, message := range messages {
		messageInfo := domainChat.MessageInfo{
			ID:           message.ID,
			ChatJID:      message.ChatJID,
			SenderJID:    message.Sender,
			Content:      message.Content,
			Timestamp:    message.Timestamp.Format(time.RFC3339),
			IsFromMe:     message.IsFromMe,
			MediaType:    message.MediaType,
			CallMetadata: message.CallMetadata,
			Filename:     message.Filename,
			URL:          message.URL,
			FileLength:   message.FileLength,
			CreatedAt:    message.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    message.UpdatedAt.Format(time.RFC3339),
		}
		if len(message.Reactions) > 0 {
			messageInfo.Reactions = make([]domainChat.ReactionInfo, 0, len(message.Reactions))
			for _, reaction := range message.Reactions {
				messageInfo.Reactions = append(messageInfo.Reactions, domainChat.ReactionInfo{
					Emoji:     reaction.Emoji,
					SenderJID: reaction.ReactorJID,
					IsFromMe:  reaction.IsFromMe,
					Timestamp: reaction.Timestamp.Format(time.RFC3339),
				})
			}
		}
		messageInfos = append(messageInfos, messageInfo)
	}

	// Create chat info for response
	chatInfo := domainChat.ChatInfo{
		JID:                 chat.JID,
		Name:                chat.Name,
		LastMessageTime:     chat.LastMessageTime.Format(time.RFC3339),
		EphemeralExpiration: chat.EphemeralExpiration,
		CreatedAt:           chat.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           chat.UpdatedAt.Format(time.RFC3339),
		Archived:            chat.Archived,
	}

	// Create pagination response
	pagination := domainChat.PaginationResponse{
		Limit:  request.Limit,
		Offset: request.Offset,
		Total:  int(totalCount),
	}

	response.Data = messageInfos
	response.Pagination = pagination
	response.ChatInfo = chatInfo

	logrus.WithFields(logrus.Fields{
		"chat_jid":       request.ChatJID,
		"total_messages": len(messageInfos),
		"limit":          request.Limit,
		"offset":         request.Offset,
	}).Info("Retrieved chat messages successfully")

	return response, nil
}

func (service serviceChat) SyncHistory(ctx context.Context, request domainChat.SyncHistoryRequest) (response domainChat.SyncHistoryResponse, err error) {
	if err = validations.ValidateSyncHistory(ctx, &request); err != nil {
		return response, err
	}

	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return response, pkgError.ErrWaCLI
	}

	// Validate JID and ensure connection
	targetJID, err := utils.ValidateJidWithLogin(client, request.ChatJID)
	if err != nil {
		return response, err
	}

	deviceID := deviceIDFromContext(ctx)
	if deviceID == "" {
		return response, fmt.Errorf("device identification required")
	}

	// On-demand history sync fetches messages immediately before a known message,
	// so we anchor the request on the oldest message we already have for this chat.
	oldest, err := service.chatStorageRepo.GetOldestMessage(deviceID, request.ChatJID)
	if err != nil {
		logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to get oldest message for history sync")
		return response, err
	}
	if oldest == nil {
		return response, fmt.Errorf("no stored messages for chat %s yet; cannot anchor history sync (open the chat to receive recent messages first)", request.ChatJID)
	}

	lastKnownInfo := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     targetJID,
			IsFromMe: oldest.IsFromMe,
			IsGroup:  targetJID.Server == types.GroupServer,
		},
		ID:        oldest.ID,
		Timestamp: oldest.Timestamp,
	}

	// Build and send the on-demand history sync request to the primary device.
	// The response arrives asynchronously as an events.HistorySync of type ON_DEMAND.
	historyMsg := client.BuildHistorySyncRequest(lastKnownInfo, request.Count)
	if _, err = client.SendPeerMessage(ctx, historyMsg); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"chat_jid": request.ChatJID,
			"count":    request.Count,
		}).Error("Failed to send history sync request")
		return response, err
	}

	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.Count = request.Count
	response.OldestMsgID = oldest.ID
	response.Message = fmt.Sprintf("Requested up to %d older messages before %s; they will be stored asynchronously when the phone responds", request.Count, oldest.Timestamp.Format(time.RFC3339))

	logrus.WithFields(logrus.Fields{
		"chat_jid":      request.ChatJID,
		"count":         request.Count,
		"oldest_msg_id": oldest.ID,
	}).Info("History sync request sent successfully")

	return response, nil
}

func (service serviceChat) RepairMedia(ctx context.Context, request domainChat.RepairMediaRequest) (response domainChat.RepairMediaResponse, err error) {
	if err = validations.ValidateRepairMedia(ctx, &request); err != nil {
		return response, err
	}

	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return response, pkgError.ErrWaCLI
	}

	// Validate JID and ensure connection
	targetJID, err := utils.ValidateJidWithLogin(client, request.ChatJID)
	if err != nil {
		return response, err
	}

	deviceID := deviceIDFromContext(ctx)
	if deviceID == "" {
		return response, fmt.Errorf("device identification required")
	}

	// Media that has a key but no direct_path can't be downloaded; ask the phone
	// to re-upload it. The fresh direct_path arrives asynchronously as an
	// events.MediaRetry and is stored by handleMediaRetry.
	messages, err := service.chatStorageRepo.GetMediaMessagesForRepair(deviceID, request.ChatJID, request.Limit)
	if err != nil {
		logrus.WithError(err).WithField("chat_jid", request.ChatJID).Error("Failed to list media messages for repair")
		return response, err
	}

	isGroup := targetJID.Server == types.GroupServer
	requested := 0
	for _, message := range messages {
		if len(message.MediaKey) == 0 {
			continue
		}

		senderJID := types.EmptyJID
		if message.Sender != "" {
			if parsed, perr := types.ParseJID(message.Sender); perr == nil {
				senderJID = parsed
			}
		}

		info := &types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     targetJID,
				Sender:   senderJID,
				IsFromMe: message.IsFromMe,
				IsGroup:  isGroup,
			},
			ID: message.ID,
		}

		if err := client.SendMediaRetryReceipt(ctx, info, message.MediaKey); err != nil {
			logrus.WithError(err).WithField("message_id", message.ID).Warn("Failed to send media retry receipt")
			continue
		}
		requested++
	}

	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.Requested = requested
	response.Message = fmt.Sprintf("Requested re-upload of %d media message(s); refreshed media will be downloadable shortly after the phone responds", requested)

	logrus.WithFields(logrus.Fields{
		"chat_jid":   request.ChatJID,
		"candidates": len(messages),
		"requested":  requested,
	}).Info("Media repair requested successfully")

	return response, nil
}

func deviceIDFromContext(ctx context.Context) string {
	if inst, ok := whatsapp.DeviceFromContext(ctx); ok && inst != nil {
		if jid := inst.JID(); jid != "" {
			return jid
		}
		return inst.ID()
	}
	return ""
}

func (service serviceChat) PinChat(ctx context.Context, request domainChat.PinChatRequest) (response domainChat.PinChatResponse, err error) {
	if err = validations.ValidatePinChat(ctx, &request); err != nil {
		return response, err
	}

	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return response, pkgError.ErrWaCLI
	}

	// Validate JID and ensure connection
	targetJID, err := utils.ValidateJidWithLogin(client, request.ChatJID)
	if err != nil {
		return response, err
	}

	// Build pin patch using whatsmeow's BuildPin
	patchInfo := appstate.BuildPin(targetJID, request.Pinned)

	// Send app state update
	if err = client.SendAppState(ctx, patchInfo); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"chat_jid": request.ChatJID,
			"pinned":   request.Pinned,
		}).Error("Failed to send pin chat app state")
		return response, err
	}

	// Build response
	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.Pinned = request.Pinned

	if request.Pinned {
		response.Message = "Chat pinned successfully"
	} else {
		response.Message = "Chat unpinned successfully"
	}

	logrus.WithFields(logrus.Fields{
		"chat_jid": request.ChatJID,
		"pinned":   request.Pinned,
	}).Info("Chat pin operation completed successfully")

	return response, nil
}

func (service serviceChat) SetDisappearingTimer(ctx context.Context, request domainChat.SetDisappearingTimerRequest) (response domainChat.SetDisappearingTimerResponse, err error) {
	if err = validations.ValidateSetDisappearingTimer(ctx, &request); err != nil {
		return response, err
	}

	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return response, pkgError.ErrWaCLI
	}

	// Validate JID and ensure connection
	targetJID, err := utils.ValidateJidWithLogin(client, request.ChatJID)
	if err != nil {
		return response, err
	}

	// Set disappearing timer using whatsmeow
	if err = client.SetDisappearingTimer(ctx, targetJID, time.Duration(request.TimerSeconds)*time.Second, time.Now()); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"chat_jid":      request.ChatJID,
			"timer_seconds": request.TimerSeconds,
		}).Error("Failed to set disappearing timer")
		return response, err
	}

	// Update local storage immediately for consistency
	if existingChat, _ := service.chatStorageRepo.GetChatByDevice(deviceIDFromContext(ctx), request.ChatJID); existingChat != nil {
		existingChat.EphemeralExpiration = request.TimerSeconds
		_ = service.chatStorageRepo.StoreChat(existingChat)
	}

	// Build response
	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.TimerSeconds = request.TimerSeconds

	if request.TimerSeconds == 0 {
		response.Message = "Disappearing messages disabled"
	} else {
		response.Message = fmt.Sprintf("Disappearing messages set to %d seconds", request.TimerSeconds)
	}

	logrus.WithFields(logrus.Fields{
		"chat_jid":      request.ChatJID,
		"timer_seconds": request.TimerSeconds,
	}).Info("Disappearing timer set successfully")

	return response, nil
}

func (service serviceChat) ArchiveChat(ctx context.Context, request domainChat.ArchiveChatRequest) (response domainChat.ArchiveChatResponse, err error) {
	if err = validations.ValidateArchiveChat(ctx, &request); err != nil {
		return response, err
	}

	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return response, pkgError.ErrWaCLI
	}

	// Validate JID and ensure connection
	targetJID, err := utils.ValidateJidWithLogin(client, request.ChatJID)
	if err != nil {
		return response, err
	}

	// Build archive patch using whatsmeow's BuildArchive
	patchInfo := appstate.BuildArchive(targetJID, request.Archived, time.Now(), nil)

	// Send app state update
	if err = client.SendAppState(ctx, patchInfo); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"chat_jid": request.ChatJID,
			"archived": request.Archived,
		}).Error("Failed to send archive chat app state")
		return response, err
	}

	// Build response
	response.Status = "success"
	response.ChatJID = request.ChatJID
	response.Archived = request.Archived

	if request.Archived {
		response.Message = "Chat archived successfully"
	} else {
		response.Message = "Chat unarchived successfully"
	}

	// Update local storage immediately for consistency
	if existingChat, _ := service.chatStorageRepo.GetChatByDevice(deviceIDFromContext(ctx), request.ChatJID); existingChat != nil {
		existingChat.Archived = request.Archived
		_ = service.chatStorageRepo.StoreChat(existingChat)
	}

	logrus.WithFields(logrus.Fields{
		"chat_jid": request.ChatJID,
		"archived": request.Archived,
	}).Info("Chat archive operation completed successfully")

	return response, nil
}
