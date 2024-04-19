package tg

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/net/proxy"
)

// MessageSent it's an alias for tgbotapi.Message
type MessageSent tgbotapi.Message

// ChatMember it's an alias for tgbotapi.ChatMember
type ChatMember tgbotapi.ChatMember

// Telegram it is a module context structure
type Telegram struct {
	bot             *tgbotapi.BotAPI
	description     Description
	usrCtx          interface{}
	redisHost       string
	updateQueueWait time.Duration
}

// Settings contains data to setting up bot
type Settings struct {
	BotSettings     SettingsBot
	RedisHost       string
	UpdateQueueWait time.Duration
}

// SettingsBot contains settings for Telegram bot
type SettingsBot struct {
	BotAPI  string
	Webhook *SettingsBotWebhook
	Proxy   *SettingsBotProxy
}

// SettingsBotWebhook contains settings to set Telegram webhook
type SettingsBotWebhook struct {
	URL      string
	BotToken string
	CertFile string
	WithCert bool
}

// SettingsBotProxy contains proxy settings for Telegram bot
type SettingsBotProxy struct {
	Type     string
	Host     string
	Login    string
	Password string
}

// Description describes bot
type Description struct {

	// Commands contains Telegram commands available for bot.
	// Each record it's a command name (without leading
	// '/' character), its description and handler
	Commands []Command

	// States contains the states description.
	// Map key it's a state name that must be set with the
	// tg.SessState() function
	States map[SessionState]State

	// InitHandler is a handler to processing Telegram updates
	// when session has not been started yet.
	// This element returns only next state.
	InitHandler func(t *Telegram, s *Session) (InitHandlerRes, error)

	// ErrorHandler is a handler called if any other handlers returned an error
	ErrorHandler func(t *Telegram, s *Session, e error) (ErrorHandlerRes, error)

	// PrimeHandler is a handler called before any user action handlers, i.e.
	// CommandHandler, InitHandler, MessageHandler, CallbackHandler.
	// If PrimeHandler returns an error, ErrorHandler will be called.
	// If PrimeHandler returns a `sessionContinue` as a new session state, following handlers
	// will be called. Otherwise session will be switched to specified state.
	PrimeHandler func(t *Telegram, s *Session, hs HandlerSource) (PrimeHandlerRes, error)

	// DestroyHandler is a handler called before session will be destroyed
	DestroyHandler func(t *Telegram, s *Session) error
}

// InitHandlerRes contains data returned by the InitHandler
type InitHandlerRes struct {

	// New state to switch the session.
	// All values of NextState must exist in States map
	// within the bot description
	NextState SessionState
}

// PrimeHandlerRes contains data returned by the PrimeHandler
type PrimeHandlerRes struct {

	// New state to switch the session.
	// All values of NextState must exist in States map
	// within the bot description.
	NextState SessionState
}

// ErrorHandlerRes contains data returned by the ErrorHandler
type ErrorHandlerRes struct {

	// New state to switch the session.
	// All values of NextState must exist in States map
	// within the bot description
	NextState SessionState
}

// StateHandlerRes contains data returned by the StateHandler
type StateHandlerRes struct {

	// Message contains message text to be sent to user.
	// Message can not be zero length
	Message string

	// ParseMode defines a Telegram message Parse mode
	ParseMode ParseMode

	// DisableWebPagePreview defines whether or not
	// disabling web page preview in messages
	DisableWebPagePreview bool

	// Buttons contains buttons for message to be sent to user.
	// If Buttons has zero length message will not contains buttons
	Buttons [][]Button

	// NextState defines next state for current session.
	// NextState will be ignored if MessageHandler defined for state
	NextState SessionState

	// Whether or not stick message. If true appropriate message will
	// be updated when a new state initiate by the `update` of callback type
	StickMessage bool
}

// MessageHandlerRes contains data returned by the MessageHandler
type MessageHandlerRes struct {

	// NextState contains next session state
	NextState SessionState
}

// CallbackHandlerRes contains data returned by the CallbackHandler
type CallbackHandlerRes struct {

	// NextState contains next session state
	NextState SessionState
}

// CommandHandlerRes contains data returned by the CommandHandler
type CommandHandlerRes struct {

	// NextState contains next session state
	NextState SessionState
}

// Command contains data for command
type Command struct {

	// Command able to execute by user (without leading
	// '/' character)
	Command string

	// Command description that users will see in Telegram
	Description string

	// Handler to processing command received from user
	Handler func(t *Telegram, s *Session, cmd string, args string) (CommandHandlerRes, error)
}

// State contains session state description
type State struct {

	// Handler to processing new bot state.
	StateHandler func(t *Telegram, s *Session) (StateHandlerRes, error)

	// Handler to processing messages received from user
	MessageHandler func(t *Telegram, s *Session) (MessageHandlerRes, error)

	// Handler to processing callbacks received from user for specific state of session
	CallbackHandler func(t *Telegram, s *Session, identifier string) (CallbackHandlerRes, error)

	// Handler to processing sent message to telegram.
	// E.g. useful for get sent messages ID
	SentHandler func(t *Telegram, s *Session, messages []MessageSent) error
}

var (
	// ErrCallbackDataFormat contains error "wrong callback data format"
	ErrCallbackDataFormat = errors.New("wrong callback data format")

	// ErrDescriptionState contains error "session state not defined in bot description"
	ErrDescriptionStateMissing = errors.New("session state not defined in bot description")

	// ErrUpdatesChanClosed contains error "updates channel has been closed"
	ErrUpdatesChanClosed = errors.New("updates channel has been closed")

	// ErrUpdateChainZeroLen contains error "update has zero len"
	ErrUpdateChainZeroLen = errors.New("update has zero len")

	// ErrUpdateWrongType contains error "update has wrong type"
	ErrUpdateWrongType = errors.New("update has wrong type")

	// ErrSessionNotExist contains error "session does not exist"
	ErrSessionNotExist = errors.New("session does not exist")
)

// Button contains buttons data for state
type Button struct {

	// Button text
	Text string

	// Defines a button identifier for processing in handler
	Identifier string

	// Defines a button mode for processing in handler ("data" (default), "url", "switch")
	Mode ButtonMode
}

// File contains file descrition received from Telegram
type File struct {
	FileSize int
	FileName string

	f tgbotapi.File
}

// FileSendStream contains options for sending file to Telegram as stream
type FileSendStream struct {
	FileType  FileType
	FileName  string
	FileSize  int64
	Caption   string
	ParseMode ParseMode
	Buttons   [][]Button
}

// FileSend contains options for sending file to Telegram
type FileSend struct {
	FileType  FileType
	FilePath  string
	Caption   string
	ParseMode ParseMode
	Buttons   [][]Button
}

// SendMessageData contains an options for message
type SendMessageData struct {

	// Message defines a message text will sent to user
	Message string

	// ParseMode defines a Telegram message Parse mode
	ParseMode ParseMode

	// DisableWebPagePreview defines whether or not
	// disabling web page preview in messages
	DisableWebPagePreview bool

	// Button defines buttons for message
	Buttons [][]Button

	// `ButtonState` set a state from bot description
	// with callback handler for spcified buttons
	ButtonState SessionState
}

// HandlerSource is a type of source handler where PrimeHandler was called
type HandlerSource string

const (
	HandlerSourceInit     HandlerSource = "init"
	HandlerSourceCommand  HandlerSource = "command"
	HandlerSourceMessage  HandlerSource = "message"
	HandlerSourceCallback HandlerSource = "callback"
)

func (hs HandlerSource) String() string {
	return string(hs)
}

// FileType specifies uploading file type
type FileType int

const (
	FileTypeDocument FileType = iota
	FileTypePhoto
	FileTypeVoice
	FileTypeVideo
	FileTypeAudio
	FileTypeSticker
)

func (f FileType) String() string {
	return [...]string{"document", "photo", "voice", "video", "audio", "sticker"}[f]
}

// ButtonMode it's a type of button mode (see https://core.telegram.org/bots/api#inlinekeyboardbutton for details)
type ButtonMode int

const (
	ButtonModeData ButtonMode = iota
	ButtonModeURL
	ButtonModeSwitch
)

func (b ButtonMode) String() string {
	return [...]string{"data", "url", "switch"}[b]
}

type ParseMode int

const (
	ParseModeMarkdown ParseMode = iota
	ParseModeMarkdownV2
	ParseModeHTML
)

func (p ParseMode) String() string {
	return [...]string{tgbotapi.ModeMarkdown, tgbotapi.ModeMarkdownV2, tgbotapi.ModeHTML}[p]
}

// Init initializes Telegram bot
func Init(s Settings, description Description, usrCtx interface{}) (Telegram, error) {

	var t Telegram

	bot, err := botConnect(s.BotSettings.BotAPI, s.BotSettings.Proxy)
	if err != nil {
		return t, err
	}

	t.bot = bot
	t.description = description
	t.usrCtx = usrCtx
	t.redisHost = s.RedisHost
	t.updateQueueWait = s.UpdateQueueWait

	if s.BotSettings.Webhook != nil {
		if err := t.webhookSet(s.BotSettings.Webhook); err != nil {
			return t, err
		}
	} else {
		if err := t.webhookDel(); err != nil {
			return t, err
		}
	}

	err = t.commandsSet()

	return t, err
}

func (t *Telegram) SelfIDGet() int64 {
	return t.bot.Self.ID
}

// Processing processes available updates from queue
func (t *Telegram) Processing() error {

	q, err := queueInit(t.redisHost, t.updateQueueWait)
	if err != nil {
		return err
	}
	defer q.close()

	// Get all available updates from queue
	uc, err := q.chainGet()
	if err != nil {
		return err
	}

	sess, err := sessionInit(uc, t.redisHost)
	if err != nil {
		if err == ErrUpdateChainZeroLen {
			return nil
		} else {
			return err
		}
	}
	defer sess.close()

	return sess.stateProcessing(t)
}

// GetUpdates creates to Telegram API and processes a receiving updates
func (t *Telegram) GetUpdates(ctx context.Context) error {

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	c := t.bot.GetUpdatesChan(u)
	defer t.bot.StopReceivingUpdates()

	for {
		select {
		case <-ctx.Done():
			return nil
		case u, b := <-c:
			if b == false {
				return ErrUpdatesChanClosed
			}
			if err := t.UpdateAbsorb(Update(u)); err != nil {
				return fmt.Errorf("bot add request into queue error: %v", err)
			}
		}
	}
}

// UpdateAbsorb absorbs specified `update` and put it into queue
func (t *Telegram) UpdateAbsorb(update Update) error {

	chatID, userID := updateIDsGet(update)

	if update.CallbackQuery != nil {
		// Do not check errors to prevent
		// `query is too old and response timeout expired or query ID is invalid` error
		t.bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
	}

	if chatID == 0 || userID == 0 {
		return nil
	}

	q, err := queueInit(t.redisHost, t.updateQueueWait)
	if err != nil {
		return err
	}
	defer q.close()

	return q.add(chatID, userID, update)
}

// UsrCtxGet gets user context
func (t *Telegram) UsrCtxGet() interface{} {
	return t.usrCtx
}

// sendMessage sends specified message to client
// Messages can be of two types: either new message, or edit existing message (if messageID is set).
func (t *Telegram) SendMessage(chatID int64, messageID int, msgData SendMessageData) ([]MessageSent, error) {

	var (
		bm  [][]tgbotapi.InlineKeyboardButton
		ikm tgbotapi.InlineKeyboardMarkup
		mr  tgbotapi.Message
		err error
	)

	// If buttons set
	if len(msgData.Buttons) > 0 {
		for _, br := range msgData.Buttons {
			var b []tgbotapi.InlineKeyboardButton
			for _, be := range br {

				d, err := callbackDataGen(msgData.ButtonState, be.Identifier)
				if err != nil {
					return []MessageSent{}, err
				}
				b = append(b, buttonPrepare(be.Text, d, be.Mode))
			}
			bm = append(bm, b)
		}

		ikm = tgbotapi.NewInlineKeyboardMarkup(bm...)
	}

	if messageID == 0 {
		msg := tgbotapi.NewMessage(chatID, msgData.Message)
		msg.ParseMode = msgData.ParseMode.String()
		msg.DisableWebPagePreview = msgData.DisableWebPagePreview

		if len(msgData.Buttons) > 0 {
			msg.ReplyMarkup = ikm
		}

		mr, err = t.bot.Send(msg)
	} else {
		msg := tgbotapi.NewEditMessageText(chatID, messageID, msgData.Message)
		msg.ParseMode = msgData.ParseMode.String()
		msg.DisableWebPagePreview = msgData.DisableWebPagePreview

		if len(msgData.Buttons) > 0 {
			msg.ReplyMarkup = &ikm
		}

		mr, err = t.bot.Send(msg)
	}

	return []MessageSent{MessageSent(mr)}, err
}

// DownloadFileStream returns io.ReadCloser to download specified file
func (t *Telegram) DownloadFileStream(file File) (io.ReadCloser, error) {

	// Make request
	req, err := http.NewRequest("GET", file.f.Link(t.bot.Token), nil)
	if err != nil {
		return nil, fmt.Errorf("can't create new request: %v", err)
	}

	// Make request
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
	client := &http.Client{Transport: tr}

	// Do request
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request error: %v", err)
	}

	if res.StatusCode == http.StatusOK {
		return res.Body, nil
	}

	res.Body.Close()

	return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
}

// DownloadFile downloads file from Telegram to specified path
func (t *Telegram) DownloadFile(file File, dstPath string) error {

	s, err := t.DownloadFileStream(file)
	if err != nil {
		return err
	}
	defer s.Close()

	lf, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer lf.Close()

	if _, err := io.Copy(lf, s); err != nil {
		return err
	}

	return nil
}

// UploadFileStream uploads file to Telegram by specified reader
func (t *Telegram) UploadFileStream(chatID int64, file FileSendStream, r io.Reader) (MessageSent, error) {

	var c tgbotapi.Chattable

	reader, ikm := uploadStreamPrepare(file, r)

	switch file.FileType {
	case FileTypePhoto:
		msg := tgbotapi.NewPhoto(chatID, reader)
		msg.ParseMode = file.ParseMode.String()
		msg.Caption = file.Caption

		if len(file.Buttons) > 0 {
			msg.ReplyMarkup = &ikm
		}
		c = msg

	case FileTypeVoice:
		msg := tgbotapi.NewVoice(chatID, reader)
		msg.ParseMode = file.ParseMode.String()
		msg.Caption = file.Caption

		if len(file.Buttons) > 0 {
			msg.ReplyMarkup = &ikm
		}
		c = msg

	case FileTypeVideo:
		msg := tgbotapi.NewVideo(chatID, reader)
		msg.ParseMode = file.ParseMode.String()
		msg.Caption = file.Caption

		if len(file.Buttons) > 0 {
			msg.ReplyMarkup = &ikm
		}
		c = msg

	case FileTypeAudio:
		msg := tgbotapi.NewAudio(chatID, reader)
		msg.ParseMode = file.ParseMode.String()
		msg.Caption = file.Caption

		if len(file.Buttons) > 0 {
			msg.ReplyMarkup = &ikm
		}
		c = msg

	case FileTypeSticker:
		msg := tgbotapi.NewSticker(chatID, reader)

		if len(file.Buttons) > 0 {
			msg.ReplyMarkup = &ikm
		}
		c = msg

	default: // including FileTypeDocument case
		// For other examples see: https://github.com/go-telegram-bot-api/telegram-bot-api/blob/master/bot_test.go
		msg := tgbotapi.NewDocument(chatID, reader)
		msg.ParseMode = file.ParseMode.String()
		msg.Caption = file.Caption

		if len(file.Buttons) > 0 {
			msg.ReplyMarkup = &ikm
		}

		c = msg
	}

	m, err := t.bot.Send(c)
	return MessageSent(m), err
}

// UploadFile uploads file as to Telegram
func (t *Telegram) UploadFile(chatID int64, file FileSend) (MessageSent, error) {

	f, err := os.Open(file.FilePath)
	if err != nil {
		return MessageSent{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return MessageSent{}, err
	}

	return t.UploadFileStream(chatID, FileSendStream{
		FileType:  file.FileType,
		FileName:  path.Base(file.FilePath),
		FileSize:  stat.Size(),
		Caption:   file.Caption,
		ParseMode: file.ParseMode,
		Buttons:   file.Buttons,
	}, f)
}

func (t *Telegram) ChatMemberGet(chatID, userID int64) (ChatMember, error) {

	c, err := t.bot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	})
	if err != nil {
		return ChatMember{}, err
	}

	return ChatMember(c), nil
}

// webhookSet sets Telegram webhook
func (t *Telegram) webhookSet(s *SettingsBotWebhook) error {

	var (
		wh  tgbotapi.WebhookConfig
		err error
	)

	if s == nil {
		return nil
	}

	// Prepare webhook URL
	whURL := s.URL
	if whURL[len(whURL)-1] != '/' {
		whURL += "/"
	}
	whURL += s.BotToken

	// Set webhook (each time when server starting)
	if s.WithCert == true {
		wh, err = tgbotapi.NewWebhookWithCert(whURL, tgbotapi.FilePath(s.CertFile))
		if err != nil {
			return fmt.Errorf("Telegram bot set webhook error: %v", err)
		}
	} else {
		wh, err = tgbotapi.NewWebhook(whURL)
		if err != nil {
			return fmt.Errorf("Telegram bot set webhook error: %v", err)
		}
	}

	if _, err := t.bot.Request(wh); err != nil {
		return fmt.Errorf("Telegram bot set webhook error: %v", err)
	}

	return nil
}

func (t *Telegram) webhookDel() error {
	if _, err := t.bot.Request(tgbotapi.DeleteWebhookConfig{}); err != nil {
		return fmt.Errorf("Telegram bot delete webhook error: %v", err)
	}
	return nil
}

// Set specified bot commands
func (t *Telegram) commandsSet() error {

	var bcmds []tgbotapi.BotCommand

	for _, c := range t.description.Commands {
		bcmds = append(bcmds, tgbotapi.BotCommand{
			Command:     c.Command,
			Description: c.Description,
		})
	}

	// Set specified commands
	if _, err := t.bot.Request(tgbotapi.NewSetMyCommands(bcmds...)); err != nil {
		return fmt.Errorf("Telegram bot set commands error: %v", err)
	}

	return nil
}

// botConnect sets up Telegram bot
func botConnect(botAPI string, p *SettingsBotProxy) (*tgbotapi.BotAPI, error) {

	if p == nil {
		return tgbotapi.NewBotAPI(botAPI)
	}

	switch p.Type {
	case "socks5":
		auth := proxy.Auth{
			User:     p.Login,
			Password: p.Password,
		}

		dialer, err := proxy.SOCKS5("tcp", p.Host, &auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("connect to proxy error: %v", err)
		}

		t := &http.Transport{Dial: dialer.Dial}

		return tgbotapi.NewBotAPIWithClient(botAPI, tgbotapi.APIEndpoint, &http.Client{Transport: t})
	}

	return nil, fmt.Errorf("unknown proxy type")
}

func (d *Description) commandLookup(cmd string) *Command {
	for _, c := range d.Commands {
		if c.Command == cmd {
			return &c
		}
	}
	return nil
}

// uploadStreamPrepare prepares reader and inline keyboard markup for stream uploading
func uploadStreamPrepare(file FileSendStream, r io.Reader) (tgbotapi.FileReader, tgbotapi.InlineKeyboardMarkup) {

	var (
		bm  [][]tgbotapi.InlineKeyboardButton
		ikm tgbotapi.InlineKeyboardMarkup
	)

	reader := tgbotapi.FileReader{
		Name:   file.FileName,
		Reader: r,
	}

	// If buttons set
	if len(file.Buttons) > 0 {
		for _, br := range file.Buttons {
			var b []tgbotapi.InlineKeyboardButton
			for _, be := range br {
				b = append(b, buttonPrepare(be.Text, be.Identifier, be.Mode))
			}
			bm = append(bm, b)
		}
		ikm = tgbotapi.NewInlineKeyboardMarkup(bm...)
	}

	return reader, ikm
}

// buttonPrepare prepare a button for inline keyboard markup
func buttonPrepare(text, identifier string, mode ButtonMode) tgbotapi.InlineKeyboardButton {
	switch mode {
	case ButtonModeURL:
		d := callbackData{}
		json.Unmarshal([]byte(identifier), &d)
		return tgbotapi.NewInlineKeyboardButtonURL(text, d.I)
	case ButtonModeSwitch:
		d := callbackData{}
		json.Unmarshal([]byte(identifier), &d)
		return tgbotapi.NewInlineKeyboardButtonSwitch(text, d.I)
	}
	return tgbotapi.NewInlineKeyboardButtonData(text, identifier)
}
