// Package whatsapp provides the WhatsApp adapter for Smara using the whatsmeow library.
package whatsapp

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"go.mau.fi/whatsmeow/proto/waE2E"
	_ "modernc.org/sqlite"

	"github.com/gede-cahya/Smara-CLI/internal/platform"
)

// Adapter implements platform.PlatformAdapter for WhatsApp.
type Adapter struct {
	client *whatsmeow.Client
	config platform.AdapterConfig
	dbLog  waLog.Logger
}

// New creates a new WhatsApp adapter.
func New() *Adapter {
	return &Adapter{
		dbLog: waLog.Stdout("Database", "WARN", true),
	}
}

// Name returns the platform identifier.
func (a *Adapter) Name() string {
	return "whatsapp"
}

// Connect initializes the WhatsApp connection and handles pairing.
func (a *Adapter) Connect(ctx context.Context, cfg platform.AdapterConfig) error {
	a.config = cfg

	// Determine session directory
	sessionDir := cfg.Extra["session_dir"]
	if sessionDir == "" {
		home, _ := os.UserHomeDir()
		sessionDir = filepath.Join(home, ".smara", "wa-session")
	}

	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return fmt.Errorf("gagal membuat direktori sesi WA: %w", err)
	}

	dbPath := filepath.Join(sessionDir, "session.db")
	container, err := sqlstore.New(ctx, "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)", a.dbLog)
	if err != nil {
		return fmt.Errorf("gagal inisialisasi database sesi WA: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("gagal mendapatkan device store: %w", err)
	}

	clientLog := waLog.Stdout("WhatsApp", "WARN", true)
	a.client = whatsmeow.NewClient(deviceStore, clientLog)

	if a.client.Store.ID == nil {
		// No saved session, need to pair
		qrChan, _ := a.client.GetQRChannel(ctx)
		err = a.client.Connect()
		if err != nil {
			return fmt.Errorf("gagal menghubungkan client: %w", err)
		}

		for evt := range qrChan {
			if evt.Event == "code" {
				// Display QR code in terminal
				fmt.Println("\n📸 SCAN QR CODE UNTUK LOGIN WHATSAPP:")
				qr, _ := qrcode.New(evt.Code, qrcode.Medium)
				fmt.Println(qr.ToSmallString(false))
				fmt.Println("Gunakan aplikasi WhatsApp di HP Anda: Settings > Linked Devices > Link a Device\n")
			} else {
				log.Printf("[whatsapp] QR Channel event: %s", evt.Event)
			}
		}
	} else {
		// Already logged in
		err = a.client.Connect()
		if err != nil {
			return fmt.Errorf("gagal menghubungkan client: %w", err)
		}
	}

	log.Printf("[whatsapp] Terhubung sebagai %s", a.client.Store.ID.String())
	return nil
}

// Listen starts handling incoming messages.
func (a *Adapter) Listen(ctx context.Context, handler platform.MessageHandler) error {
	if a.client == nil {
		return fmt.Errorf("client belum terhubung")
	}

	a.client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			// Only handle text messages for now
			content := v.Message.GetConversation()
			if content == "" {
				content = v.Message.GetExtendedTextMessage().GetText()
			}

			if content == "" {
				return
			}

			msg := platform.IncomingMessage{
				ID:        v.Info.ID,
				Platform:  "whatsapp",
				ChannelID: v.Info.Chat.String(),
				UserID:    v.Info.Sender.String(),
				Username:  v.Info.PushName,
				Content:   content,
				Timestamp: v.Info.Timestamp,
				Metadata:  make(map[string]string),
			}

			// Simple command parsing (WhatsApp often uses / or just text)
			if strings.HasPrefix(content, "/") {
				parts := strings.Fields(content[1:])
				if len(parts) > 0 {
					msg.IsCommand = true
					msg.Command = parts[0]
					msg.CommandArgs = parts[1:]
				}
			}

			go func() {
				if err := handler(ctx, msg); err != nil {
					log.Printf("[whatsapp] Error handling message: %v", err)
				}
			}()
		}
	})

	// Keep alive until context is done
	<-ctx.Done()
	return nil
}

// SendMessage sends a text message to a WhatsApp contact/group.
func (a *Adapter) SendMessage(ctx context.Context, channelID string, msg platform.OutgoingMessage) error {
	if a.client == nil {
		return fmt.Errorf("client belum terhubung")
	}

	jid, err := types.ParseJID(channelID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	waMsg := &waE2E.Message{
		Conversation: proto.String(msg.Content),
	}

	_, err = a.client.SendMessage(ctx, jid, waMsg)
	if err != nil {
		return fmt.Errorf("gagal mengirim pesan WA: %w", err)
	}

	return nil
}

// SendTyping sends a typing indicator (composing).
func (a *Adapter) SendTyping(ctx context.Context, channelID string) error {
	if a.client == nil {
		return nil
	}

	jid, err := types.ParseJID(channelID)
	if err != nil {
		return nil
	}

	_ = a.client.SendChatPresence(ctx, jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	return nil
}

// Close disconnects the WhatsApp client.
func (a *Adapter) Close() error {
	if a.client != nil {
		a.client.Disconnect()
	}
	return nil
}
