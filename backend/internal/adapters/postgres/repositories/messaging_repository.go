package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/sx110903/llmatch-v2/backend/internal/adapters/postgres/sqlc"
	domainmessaging "github.com/sx110903/llmatch-v2/backend/internal/domain/messaging"
)

type MessagingRepository struct {
	queries *db.Queries
}

func NewMessagingRepository(pool *pgxpool.Pool) *MessagingRepository {
	return &MessagingRepository{queries: db.New(pool)}
}

func (r *MessagingRepository) GetActiveParticipants(ctx context.Context, matchID, viewerID uuid.UUID) (*domainmessaging.Participants, error) {
	row, err := r.queries.GetActiveParticipants(ctx, db.GetActiveParticipantsParams{
		MatchID:  toPgUUID(matchID),
		ViewerID: toPgUUID(viewerID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainmessaging.ErrMatchNotAccessible
		}
		return nil, fmt.Errorf("get active participants: %w", err)
	}
	return &domainmessaging.Participants{
		MatchID:    fromPgUUID(row.ID),
		UserLowID:  fromPgUUID(row.UserLowID),
		UserHighID: fromPgUUID(row.UserHighID),
		MatchedAt:  fromPgTimestamptz(row.MatchedAt),
	}, nil
}

// Send relies on the (sender_id, client_nonce) unique index for idempotency:
// the insert simply does nothing on conflict and the stored row is returned
// with Created=false, so a retry and a race both converge on one message.
func (r *MessagingRepository) Send(ctx context.Context, message *domainmessaging.Message) (*domainmessaging.SendResult, error) {
	params := db.InsertMessageParams{
		ID:          toPgUUID(message.ID),
		MatchID:     toPgUUID(message.MatchID),
		SenderID:    toPgUUID(message.SenderID),
		ClientNonce: toPgUUID(message.ClientNonce),
		Type:        string(message.Type),
		Content:     toPgText(message.Content),
		StorageKey:  toPgText(message.StorageKey),
	}

	row, err := r.queries.InsertMessage(ctx, params)
	if err == nil {
		return &domainmessaging.SendResult{Message: messageFromRow(row), Created: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	existing, err := r.queries.GetMessageByNonce(ctx, db.GetMessageByNonceParams{
		SenderID:    toPgUUID(message.SenderID),
		ClientNonce: toPgUUID(message.ClientNonce),
	})
	if err != nil {
		return nil, fmt.Errorf("load message by nonce: %w", err)
	}
	return &domainmessaging.SendResult{Message: messageFromRow(existing), Created: false}, nil
}

func (r *MessagingRepository) ListHistory(ctx context.Context, params domainmessaging.HistoryParams) ([]domainmessaging.Message, error) {
	query := db.ListMessagesBeforeParams{
		MatchID:   toPgUUID(params.MatchID),
		PageLimit: int32(params.Limit), // #nosec G115 -- the service clamps the page size
	}
	if params.Before != nil {
		query.CursorCreatedAt = toPgTimestamptz(params.Before.CreatedAt)
		query.CursorMessageID = toPgUUID(params.Before.MessageID)
	}

	rows, err := r.queries.ListMessagesBefore(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	messages := make([]domainmessaging.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, messageFromRow(row))
	}
	return messages, nil
}

func (r *MessagingRepository) MarkRead(ctx context.Context, matchID, viewerID uuid.UUID, at time.Time) (int, error) {
	affected, err := r.queries.MarkMessagesRead(ctx, db.MarkMessagesReadParams{
		MatchID:  toPgUUID(matchID),
		ViewerID: toPgUUID(viewerID),
		ReadAt:   toPgTimestamptz(at),
	})
	if err != nil {
		return 0, fmt.Errorf("mark messages read: %w", err)
	}
	return int(affected), nil
}

func (r *MessagingRepository) ListConversations(ctx context.Context, viewerID uuid.UUID, limit int) ([]domainmessaging.ConversationSummary, error) {
	rows, err := r.queries.ListConversations(ctx, db.ListConversationsParams{
		ViewerID:  toPgUUID(viewerID),
		PageLimit: int32(limit), // #nosec G115 -- the service clamps the page size
	})
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}

	summaries := make([]domainmessaging.ConversationSummary, 0, len(rows))
	for _, row := range rows {
		summary := domainmessaging.ConversationSummary{
			MatchID:        fromPgUUID(row.MatchID),
			OtherUserID:    fromPgUUID(row.OtherUserID),
			DisplayName:    fromPgText(row.DisplayName),
			PrimaryPhotoID: fromPgUUID(row.PrimaryPhotoID),
			UnreadCount:    int(row.UnreadCount),
			MatchedAt:      fromPgTimestamptz(row.MatchedAt),
		}
		if row.LastMessageID.Valid {
			summary.LastMessage = &domainmessaging.Message{
				ID:        fromPgUUID(row.LastMessageID),
				MatchID:   fromPgUUID(row.MatchID),
				SenderID:  fromPgUUID(row.LastMessageSenderID),
				Type:      domainmessaging.MessageType(row.LastMessageType),
				Content:   fromPgText(row.LastMessageContent),
				ReadAt:    fromPgTimestamptzPtr(row.LastMessageReadAt),
				CreatedAt: fromPgTimestamptz(row.LastMessageCreatedAt),
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func messageFromRow(row db.Message) domainmessaging.Message {
	return domainmessaging.Message{
		ID:          fromPgUUID(row.ID),
		MatchID:     fromPgUUID(row.MatchID),
		SenderID:    fromPgUUID(row.SenderID),
		ClientNonce: fromPgUUID(row.ClientNonce),
		Type:        domainmessaging.MessageType(row.Type),
		Content:     fromPgText(row.Content),
		StorageKey:  fromPgText(row.StorageKey),
		ReadAt:      fromPgTimestamptzPtr(row.ReadAt),
		CreatedAt:   fromPgTimestamptz(row.CreatedAt),
		DeletedAt:   fromPgTimestamptzPtr(row.DeletedAt),
	}
}
