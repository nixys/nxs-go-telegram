package tg

import (
	"path"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Update is an update response, from Telegram GetUpdates.
type Update tgbotapi.Update

// UpdateType is a type of update chain
type UpdateType int

// UpdateChain contains chain of updates
type UpdateChain struct {
	updateType UpdateType
	chatID     int64
	userID     int64
	userName   string

	updates []Update
}

const (

	// UpdateTypeNone - type `none` for update chain.
	// No type has not been set yet or chain has been cleaned up.
	UpdateTypeNone UpdateType = iota

	// UpdateTypeUnknown - unknown update type
	UpdateTypeUnknown

	// UpdateTypeMessage - type message
	UpdateTypeMessage

	// UpdateTypeCallback - type callback
	UpdateTypeCallback
)

func (u UpdateType) String() string {
	return [...]string{"none", "unknown", "message", "callback"}[u]
}

// Get gets all updates from chain
func (uc *UpdateChain) Get() []Update {
	return uc.updates
}

// MessageTextGet gets messages text or captions for every update from chain.
// Chain must have message type
func (uc *UpdateChain) MessageTextGet() []string {

	var text []string

	if uc.updateType != UpdateTypeMessage {
		return text
	}

	for _, u := range uc.updates {

		if u.Message != nil {

			if len(u.Message.Text) > 0 {
				text = append(text, u.Message.Text)
			} else if len(u.Message.Caption) > 0 {
				text = append(text, u.Message.Caption)
			}
		}
	}

	return text
}

// MessagesIDGet gets update ids if chain has message type
func (uc *UpdateChain) MessagesIDGet() []int64 {

	var ids []int64

	if uc.updateType != UpdateTypeMessage {
		return ids
	}

	for _, u := range uc.updates {
		ids = append(ids, int64(u.Message.MessageID))
	}

	return ids
}

// CallbackQueryIDGet gets callback ID from first update element from chain.
// Chain must have callback type
func (uc *UpdateChain) CallbackQueryIDGet() string {

	if uc.updateType != UpdateTypeCallback {
		return ""
	}

	if len(uc.updates) == 0 {
		return ""
	}

	return uc.updates[0].CallbackQuery.ID
}

// FilesGet gets files from update chain.
// At the time only Photo, Document and Voice types are supported
func (uc *UpdateChain) FilesGet(t Telegram) []File {

	var files []File

	if uc.updateType != UpdateTypeMessage {
		return files

	}

	for _, u := range uc.updates {

		if elt := u.Message.Photo; len(elt) > 0 {
			// Get last element in array (largest by size)
			e := elt[len(elt)-1]

			f, err := t.bot.GetFile(tgbotapi.FileConfig{
				FileID: e.FileID,
			})
			if err != nil {
				return files
			}

			files = append(files, File{
				FileSize: f.FileSize,
				FileName: path.Base(f.FilePath),
				f:        f,
			})
		}

		if elt := u.Message.Document; elt != nil {
			f, err := t.bot.GetFile(tgbotapi.FileConfig{
				FileID: (*elt).FileID,
			})
			if err != nil {
				return files
			}

			files = append(files, File{
				FileSize: f.FileSize,
				FileName: elt.FileName,
				f:        f,
			})
		}

		if elt := u.Message.Voice; elt != nil {
			f, err := t.bot.GetFile(tgbotapi.FileConfig{
				FileID: (*elt).FileID,
			})
			if err != nil {
				return files
			}

			files = append(files, File{
				FileSize: f.FileSize,
				FileName: path.Base(f.FilePath),
				f:        f,
			})
		}
	}

	return files
}

// TypeGet gets chain type
func (uc *UpdateChain) TypeGet() UpdateType {
	return uc.updateType
}

// ChatIDGet gets chain chat ID
func (uc *UpdateChain) ChatIDGet() int64 {
	return uc.chatID
}

// UserIDGet gets chain user ID
func (uc *UpdateChain) UserIDGet() int64 {
	return uc.userID
}

// UserNameGet gets chain user name
func (uc *UpdateChain) UserNameGet() string {
	return uc.userName
}

// add adds new updates into update chain
func (uc *UpdateChain) add(chatID, userID int64, updates []Update) {

	for _, u := range updates {

		t := updateTypeEltGet(u)

		if t == UpdateTypeUnknown {
			continue
		}

		// If chain has no type yet
		if uc.updateType == UpdateTypeNone {
			uc.updateType = t
			uc.chatID = chatID
			uc.userID = userID
			uc.userName = updateUserNameGet(u)
		}

		// Skip new elements with different type
		if uc.updateType != t {
			continue
		}

		// Skip new elements with different chatID or userID
		cID, uID := updateIDsGet(u)
		if uc.chatID != cID || uc.userID != uID {
			continue
		}

		// Add new element into chain
		uc.updates = append(uc.updates, u)
	}
}

// callbackDataGet gets callback data from first update element from chain.
// Chain must have callback type
func (uc *UpdateChain) callbackDataGet() string {

	if uc.updateType != UpdateTypeCallback {
		return ""
	}

	if len(uc.updates) == 0 {
		return ""
	}

	return uc.updates[0].CallbackQuery.Data
}

// commandCheck checks first update element in chain has command signs.
// If so command and its args will be returned.
// Chain must have message type
func (uc *UpdateChain) commandCheck() (string, string) {

	if uc.updateType != UpdateTypeMessage {
		return "", ""
	}

	if len(uc.updates) == 0 {
		return "", ""
	}

	update := uc.updates[0]

	return update.Message.Command(), update.Message.CommandArguments()
}

// updateTypeEltGet gets type for specified update element
func updateTypeEltGet(update Update) UpdateType {

	if update.Message != nil {
		return UpdateTypeMessage
	}

	if update.CallbackQuery != nil {
		return UpdateTypeCallback
	}

	return UpdateTypeUnknown
}

// updateIDsGet gets chat and user ID from specified update element
func updateIDsGet(update Update) (int64, int64) {

	switch updateTypeEltGet(update) {
	case UpdateTypeMessage:
		return update.Message.Chat.ID, update.Message.From.ID
	case UpdateTypeCallback:
		return update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.From.ID
	}

	return 0, 0
}

// updateUserNameGet gets user name from specified update element
func updateUserNameGet(update Update) string {

	switch updateTypeEltGet(update) {
	case UpdateTypeMessage:
		return update.Message.From.UserName
	case UpdateTypeCallback:
		return update.CallbackQuery.From.UserName
	}

	return ""
}
