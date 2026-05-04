package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dragonspirit/db"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	usersJSONPath = "/home/lampac/users.json"
	stateJSONPath = "/home/lampac/state.json"
	whitelistPath = "/home/lampac/whitelist.txt"
)

type User struct {
	ID      string     `json:"id"`
	Group   int        `json:"group"`
	Expires time.Time  `json:"expires"`
	Comment string     `json:"comment"`
	Params  UserParams `json:"params"`
}

type UserParams struct {
	Adult bool `json:"adult"`
        Tv    bool `json:"tv"`
	Admin bool `json:"admin"`
}

type StateInfo struct {
	ChatID      int64  `json:"chatId"`
	State       string `json:"state"`
	TempUser    *User  `json:"tempUser,omitempty"`
	EditUserID  string `json:"editUserId,omitempty"`
	EditAction  string `json:"editAction,omitempty"`
	MessageID   int    `json:"messageId,omitempty"`
	MainMsgID   int    `json:"mainMsgId,omitempty"`
	TempMsgID   int    `json:"tempMsgId,omitempty"`
}

var adminIDs map[int64]bool = make(map[int64]bool)
var allowedUserIDs map[int64]bool = make(map[int64]bool)

// ========== WHITELIST ==========
func loadWhitelist() error {
	data, err := os.ReadFile(whitelistPath)
	if err != nil {
		if os.IsNotExist(err) {
			allowedUserIDs = make(map[int64]bool)
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id, err := strconv.ParseInt(line, 10, 64)
		if err == nil {
			allowedUserIDs[id] = true
		}
	}
	return nil
}

func saveWhitelist() error {
	var lines []string
	for id := range allowedUserIDs {
		lines = append(lines, fmt.Sprintf("%d", id))
	}
	return os.WriteFile(whitelistPath, []byte(strings.Join(lines, "\n")), 0644)
}

func isAllowed(chatID int64) bool {
	if isAdmin(chatID) {
		return true
	}
	return allowedUserIDs[chatID]
}

func isAdmin(chatID int64) bool {
	return adminIDs[chatID]
}

func addToWhitelist(chatID int64) error {
	allowedUserIDs[chatID] = true
	return saveWhitelist()
}

func removeFromWhitelist(chatID int64) error {
	delete(allowedUserIDs, chatID)
	return saveWhitelist()
}

// ========== USERS ==========
func loadUsers() ([]User, error) {
	data, err := os.ReadFile(usersJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []User{}, nil
		}
		return nil, err
	}
	var users []User
	err = json.Unmarshal(data, &users)
	return users, err
}

func saveUsers(users []User) error {
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(usersJSONPath, data, 0644)
}

// ========== STATES ==========
func loadStates() ([]StateInfo, error) {
	data, err := os.ReadFile(stateJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []StateInfo{}, nil
		}
		return nil, err
	}
	var states []StateInfo
	err = json.Unmarshal(data, &states)
	return states, err
}

func saveStates(states []StateInfo) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateJSONPath, data, 0644)
}

func updateMainMsgID(chatID int64, msgID int) {
	states, _ := loadStates()
	for i := range states {
		if states[i].ChatID == chatID {
			states[i].MainMsgID = msgID
			saveStates(states)
			return
		}
	}
	states = append(states, StateInfo{ChatID: chatID, MainMsgID: msgID, State: "start"})
	saveStates(states)
}

func getMainMsgID(chatID int64) int {
	states, _ := loadStates()
	for i := range states {
		if states[i].ChatID == chatID {
			return states[i].MainMsgID
		}
	}
	return 0
}

// ========== HELPERS ==========
func escapeHTML(text string) string {
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(text)
}

func escapeMarkdown(text string) string {
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

func generatePassword(telegramID int64) string {
	return fmt.Sprintf("%d", telegramID)
}

func findUserByTelegramID(users []User, telegramID int64) *User {
	idStr := fmt.Sprintf("%d", telegramID)
	for i := range users {
		if users[i].ID == idStr {
			return &users[i]
		}
	}
	return nil
}

func autoCreateUser(telegramID int64) (string, error) {
	users, err := loadUsers()
	if err != nil {
		return "", err
	}

	if findUserByTelegramID(users, telegramID) != nil {
		return generatePassword(telegramID), nil
	}

	newUser := User{
		ID:      fmt.Sprintf("%d", telegramID),
		Group:   1,
		Expires: time.Now().AddDate(1, 0, 0),
		Comment: fmt.Sprintf("Автоматически создан из Telegram ID %d", telegramID),
		Params:  UserParams{Adult: false, Tv: false, Admin: false},
	}

	users = append(users, newUser)
	if err := saveUsers(users); err != nil {
		return "", err
	}

	log.Printf("✅ Автоматически создан пользователь %s", newUser.ID)
	return generatePassword(telegramID), nil
}

// ========== INLINE KEYBOARDS ==========
func getMainMenuKeyboard() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🆕 Новый пользователь", "menu:new_user"),
			tgbotapi.NewInlineKeyboardButtonData("👥 Пользователи", "menu:list_users"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить в белый список", "menu:add_whitelist"),
			tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить из белого списка", "menu:remove_whitelist"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Белый список", "menu:show_whitelist"),
		),
	)
	return &kb
}

func getCancelInlineKeyboard() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", "cancel"),
		),
	)
	return &kb
}

func getGroupInlineKeyboard() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1️⃣ Группа 1", "group:1"),
			tgbotapi.NewInlineKeyboardButtonData("2️⃣ Группа 2", "group:2"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", "cancel"),
		),
	)
	return &kb
}

func getExpiresInlineKeyboard() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📆 6 месяцев", "expires:6m"),
			tgbotapi.NewInlineKeyboardButtonData("📆 1 год", "expires:1y"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", "cancel"),
		),
	)
	return &kb
}

func getCommentInlineKeyboard() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💬 Без комментария", "comment:none"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", "cancel"),
		),
	)
	return &kb
}

func getEditInlineKeyboard(userID string) *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏷 Группа", fmt.Sprintf("edit:group:%s", userID)),
			tgbotapi.NewInlineKeyboardButtonData("📅 Срок", fmt.Sprintf("edit:expires:%s", userID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💬 Комментарий", fmt.Sprintf("edit:comment:%s", userID)),
		),
                tgbotapi.NewInlineKeyboardRow(
                        tgbotapi.NewInlineKeyboardButtonData("🔞 Adult", fmt.Sprintf("edit:adult:%s", userID)),
                        tgbotapi.NewInlineKeyboardButtonData("📺 TV", fmt.Sprintf("edit:tv:%s", userID)),
                        tgbotapi.NewInlineKeyboardButtonData("👑 Admin", fmt.Sprintf("edit:admin:%s", userID)),
                ),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", "cancel"),
		),
	)
	return &kb
}

// ========== MAIN MENU ==========
func updateMainMenu(chatID int64, bot *tgbotapi.BotAPI, messageID int) {
	telegramID := chatID

	code, err := db.GenerateAndSaveCodeIntoDb(telegramID)
	if err != nil {
		log.Println("Ошибка получения кода:", err)
		code = "ошибка"
	}

	text := fmt.Sprintf(
		"👋 *Администратор*\n\n📌 *Ваш Telegram ID:* `%d`\n🔑 *Ваш пароль для входа:* `%d`\n🔑 *Ваш код для синхронизации:* `%s`\n\nВведите в настройках аккаунта Lampa.",
		telegramID, telegramID, code,
	)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = getMainMenuKeyboard()
	sentMsg, _ := bot.Send(editMsg)
	
	updateMainMsgID(chatID, sentMsg.MessageID)
}

// ========== USER LIST ==========
func handleListUsers(chatID int64, bot *tgbotapi.BotAPI, messageID int) {
	users, err := loadUsers()
	if err != nil || len(users) == 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "📭 Список пользователей пуст")
		editMsg.ParseMode = "Markdown"
		sentMsg, _ := bot.Send(editMsg)
		updateMainMsgID(chatID, sentMsg.MessageID)
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, user := range users {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("👤 %s", escapeMarkdown(user.ID)), fmt.Sprintf("select_user:%s", user.ID)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Назад в меню", "back_to_menu"),
	))

	text := "📋 *Список пользователей*\n\nВыберите пользователя для просмотра и управления:"
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	editMsg.ReplyMarkup = &kb
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

// ========== USER DETAIL ==========
func handleUserDetail(chatID int64, bot *tgbotapi.BotAPI, userID string, messageID int) {
	users, err := loadUsers()
	if err != nil {
		return
	}

	var user *User
	for i := range users {
		if users[i].ID == userID {
			user = &users[i]
			break
		}
	}
	if user == nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "⚠️ Пользователь не найден")
		editMsg.ParseMode = "Markdown"
		sentMsg, _ := bot.Send(editMsg)
		updateMainMsgID(chatID, sentMsg.MessageID)
		return
	}

	expiresDate := user.Expires.Format("02.01.2006")
	syncCode := "❌ нет"
	userIDint, err := strconv.ParseInt(user.ID, 10, 64)
	if err == nil && userIDint > 0 {
		code, err := db.GenerateAndSaveCodeIntoDb(userIDint)
		if err == nil && code != "" {
			syncCode = code
		}
	}

	text := fmt.Sprintf(
		"👤 *%s*\n🏷 Группа: `%d`\n📅 Доступ до: `%s`\n💬 Комментарий: `%s`\n🔞 Adult: %v | 📺 TV: %v | 👑 Admin: %v\n🔑 Пароль: `%s`\n🔑 Код: `%s`",
		escapeMarkdown(user.ID), user.Group, expiresDate, escapeMarkdown(user.Comment),
		user.Params.Adult, user.Params.Tv, user.Params.Admin, escapeMarkdown(user.ID), syncCode,
	)

	inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ Редактировать", fmt.Sprintf("edit_user:%s", user.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("delete_user:%s", user.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Назад к списку", "back_to_user_list"),
		),
	)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &inlineKeyboard
	sentMsg, _ := bot.Send(editMsg)
	
	states, _ := loadStates()
	for i := range states {
		if states[i].ChatID == chatID {
			states[i].MainMsgID = sentMsg.MessageID
			saveStates(states)
			break
		}
	}
}

// ========== WHITELIST SHOW ==========
func handleShowWhitelistInline(chatID int64, bot *tgbotapi.BotAPI, messageID int, callbackID string) {
	if len(allowedUserIDs) == 0 {
		callback := tgbotapi.NewCallback(callbackID, "📭 Белый список пуст")
		bot.Send(callback)
		return
	}

	var text string = "📋 *Белый список Telegram ID:*\n"
	for id := range allowedUserIDs {
		text += fmt.Sprintf("• `%d`\n", id)
	}

	inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Назад в меню", "back_to_menu"),
		),
	)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &inlineKeyboard
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

// ========== CALLBACK QUERY ==========
func handleCallbackQuery(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
	log.Printf("🔔 ПОЛУЧЕН CALLBACK: %s", callback.Data)

	chatID := callback.Message.Chat.ID
	msgID := callback.Message.MessageID
	data := callback.Data

	if data == "cancel" {
		states, _ := loadStates()
		var currentState *StateInfo
		for i := range states {
			if states[i].ChatID == chatID {
				currentState = &states[i]
				break
			}
		}

		if currentState != nil && currentState.State == "edit_menu" {
			handleUserDetail(chatID, bot, currentState.EditUserID, msgID)
			currentState.State = "start"
			currentState.EditUserID = ""
			currentState.MainMsgID = 0
			saveStates(states)
			return
		}

		if currentState != nil && (currentState.State == "edit_wait_group" ||
			currentState.State == "edit_wait_expires" ||
			currentState.State == "edit_wait_comment") {
			handleUserDetail(chatID, bot, currentState.EditUserID, msgID)
			currentState.State = "start"
			currentState.TempMsgID = 0
			currentState.MainMsgID = 0
			saveStates(states)
			return
		}

		if currentState != nil && currentState.State == "add_whitelist" {
			telegramID := chatID
			code, err := db.GenerateAndSaveCodeIntoDb(telegramID)
			if err != nil {
				log.Println("Ошибка получения кода:", err)
				code = "ошибка"
			}
			text := fmt.Sprintf(
				"👋 *Администратор*\n\n📌 *Ваш Telegram ID:* `%d`\n🔑 *Ваш пароль для входа:* `%d`\n🔑 *Ваш код для синхронизации:* `%s`\n\nВведите в настройках аккаунта Lampa.",
				telegramID, telegramID, code,
			)
			editMsg := tgbotapi.NewEditMessageText(chatID, msgID, text)
			editMsg.ParseMode = "Markdown"
			editMsg.ReplyMarkup = getMainMenuKeyboard()
			sentMsg, _ := bot.Send(editMsg)
			updateMainMsgID(chatID, sentMsg.MessageID)
			currentState.State = "start"
			currentState.TempUser = nil
			saveStates(states)
			return
		}

		updateMainMenu(chatID, bot, msgID)
		return
	}

	if data == "back_to_menu" {
		updateMainMenu(chatID, bot, msgID)
		return
	}

	if data == "back_to_user_list" {
		handleListUsers(chatID, bot, msgID)
		return
	}

	if data == "comment:none" {
		states, _ := loadStates()
		var currentState *StateInfo
		for i := range states {
			if states[i].ChatID == chatID {
				currentState = &states[i]
				break
			}
		}

		if currentState != nil && currentState.TempUser != nil && currentState.State == "newUser_comment" {
			bot.Send(tgbotapi.NewDeleteMessage(chatID, msgID))

			currentState.TempUser.Comment = ""

			users, _ := loadUsers()
			users = append(users, *currentState.TempUser)
			saveUsers(users)

			userIDint, err := strconv.ParseInt(currentState.TempUser.ID, 10, 64)
			var syncCode string
			if err == nil && userIDint > 0 {
				code, err := db.GenerateAndSaveCodeIntoDb(userIDint)
				if err == nil {
					syncCode = code
				}
			}

			resultText := fmt.Sprintf("✅ *Пользователь добавлен!*\n🆔 ID: `%s`\n🏷 Группа: `%d`\n📅 Доступ до: `%s`\n💬 Комментарий: `%s`\n🔞 Adult: false | 📺 TV: false | 👑 Admin: false\n🔑 Пароль: `%s`\n🔑 Код синхронизации: `%s`",
				escapeMarkdown(currentState.TempUser.ID), currentState.TempUser.Group,
				currentState.TempUser.Expires.Format("02.01.2006"), "",
				escapeMarkdown(currentState.TempUser.ID), syncCode)

			inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("✏️ Редактировать", fmt.Sprintf("edit_user:%s", currentState.TempUser.ID)),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("delete_user:%s", currentState.TempUser.ID)),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("◀️ Назад к списку", "back_to_user_list"),
				),
			)

			newMsg := tgbotapi.NewMessage(chatID, resultText)
			newMsg.ParseMode = "Markdown"
			newMsg.ReplyMarkup = &inlineKeyboard
			sentMsg, _ := bot.Send(newMsg)

			for i := range states {
				if states[i].ChatID == chatID {
					states[i].MainMsgID = sentMsg.MessageID
					break
				}
			}

			currentState.TempUser = nil
			currentState.State = "start"
			saveStates(states)
			return
		}

		if currentState != nil && currentState.State == "edit_wait_comment" {
			users, _ := loadUsers()
			for i := range users {
				if users[i].ID == currentState.EditUserID {
					users[i].Comment = ""
					saveUsers(users)
					break
				}
			}

			handleUserDetail(chatID, bot, currentState.EditUserID, msgID)

			currentState.State = "start"
			currentState.TempMsgID = 0
			saveStates(states)
		}
		return
	}

	if strings.HasPrefix(data, "menu:") {
		action := strings.TrimPrefix(data, "menu:")
		switch action {
		case "new_user":
			handleNewUserStart(chatID, bot, msgID)
		case "list_users":
			handleListUsers(chatID, bot, msgID)
		case "add_whitelist":
			handleAddWhitelistStart(chatID, bot, msgID)
		case "remove_whitelist":
			handleRemoveWhitelistStart(chatID, bot, msgID, callback.ID)
		case "show_whitelist":
			handleShowWhitelistInline(chatID, bot, msgID, callback.ID)
		}
		return
	}

	if strings.HasPrefix(data, "select_user:") {
		userID := strings.TrimPrefix(data, "select_user:")
		handleUserDetail(chatID, bot, userID, msgID)
		return
	}

	if strings.HasPrefix(data, "delete_user:") {
		userID := strings.TrimPrefix(data, "delete_user:")
		showDeleteConfirmation(chatID, bot, msgID, userID)
		return
	}

	if strings.HasPrefix(data, "confirm_delete:") {
		userID := strings.TrimPrefix(data, "confirm_delete:")
		confirmDeleteUser(chatID, bot, msgID, userID)
		return
	}

	if strings.HasPrefix(data, "edit_user:") {
		userID := strings.TrimPrefix(data, "edit_user:")
		startEditUser(chatID, bot, msgID, userID)
		return
	}

	if strings.HasPrefix(data, "whitelist_remove:") {
		idStr := strings.TrimPrefix(data, "whitelist_remove:")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil {
			delete(allowedUserIDs, id)
			saveWhitelist()

			notifyMsg, _ := bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Пользователь %d удалён из белого списка", id)))

			time.AfterFunc(3*time.Second, func() {
				bot.Send(tgbotapi.NewDeleteMessage(chatID, notifyMsg.MessageID))
			})
		}
		updateMainMenu(chatID, bot, msgID)
		return
	}

	if strings.HasPrefix(data, "group:") {
		groupStr := strings.TrimPrefix(data, "group:")
		var group int
		if groupStr == "1" {
			group = 1
		} else if groupStr == "2" {
			group = 2
		} else {
			group, _ = strconv.Atoi(groupStr)
		}
		states, _ := loadStates()
		for i := range states {
			if states[i].ChatID == chatID {
				if states[i].TempUser != nil {
					states[i].TempUser.Group = group
					states[i].State = "newUser_expires"
					saveStates(states)
				}
				break
			}
		}
		showExpiresInput(chatID, bot, msgID)
		return
	}

	if strings.HasPrefix(data, "expires:") {
		expiresOption := strings.TrimPrefix(data, "expires:")
		var expires time.Time
		if expiresOption == "6m" {
			expires = time.Now().AddDate(0, 6, 0)
		} else if expiresOption == "1y" {
			expires = time.Now().AddDate(1, 0, 0)
		}
		states, _ := loadStates()
		var currentState *StateInfo
		for i := range states {
			if states[i].ChatID == chatID {
				currentState = &states[i]
				break
			}
		}

		if currentState != nil && currentState.State == "edit_wait_expires" {
			users, _ := loadUsers()
			for i := range users {
				if users[i].ID == currentState.EditUserID {
					users[i].Expires = expires
					saveUsers(users)
					break
				}
			}

			handleUserDetail(chatID, bot, currentState.EditUserID, msgID)

			currentState.State = "start"
			currentState.TempMsgID = 0
			saveStates(states)
			return
		}

		states, _ = loadStates()
		for i := range states {
			if states[i].ChatID == chatID && states[i].TempUser != nil {
				states[i].TempUser.Expires = expires
				states[i].State = "newUser_comment"
				saveStates(states)
				break
			}
		}

		showCommentInput(chatID, bot, msgID)
		return
	}

	if strings.HasPrefix(data, "comment:") && data != "comment:none" {
		comment := strings.TrimPrefix(data, "comment:")
		states, _ := loadStates()
		var tempUser *User
		for i := range states {
			if states[i].ChatID == chatID {
				tempUser = states[i].TempUser
				break
			}
		}
		if tempUser != nil {
			if comment != "none" {
				tempUser.Comment = comment
			}
			users, _ := loadUsers()
			users = append(users, *tempUser)
			saveUsers(users)

			userIDint, err := strconv.ParseInt(tempUser.ID, 10, 64)
			var syncCode string
			if err == nil && userIDint > 0 {
				code, err := db.GenerateAndSaveCodeIntoDb(userIDint)
				if err == nil {
					syncCode = code
				}
			}

			for i := range states {
				if states[i].ChatID == chatID {
					states[i].TempUser = nil
					states[i].State = "start"
					break
				}
			}
			saveStates(states)

			text := fmt.Sprintf("✅ *Пользователь добавлен!*\n🆔 ID: `%s`\n🏷 Группа: `%d`\n📅 Доступ до: `%s`\n💬 Комментарий: `%s`\n🔞 Adult: false | 📺 TV: false | 👑 Admin: false\n🔑 Пароль: `%s`\n🔑 Код синхронизации: `%s`",
				escapeMarkdown(tempUser.ID), tempUser.Group, tempUser.Expires.Format("02.01.2006"),
				escapeMarkdown(tempUser.Comment), escapeMarkdown(tempUser.ID), syncCode)
			editMsg := tgbotapi.NewEditMessageText(chatID, msgID, text)
			editMsg.ParseMode = "Markdown"
			backKb := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("◀️ Назад в меню", "back_to_menu"),
				),
			)
			editMsg.ReplyMarkup = &backKb
			sentMsg, _ := bot.Send(editMsg)
			updateMainMsgID(chatID, sentMsg.MessageID)
		}
		return
	}

	if strings.HasPrefix(data, "edit:group:") {
		userID := strings.TrimPrefix(data, "edit:group:")
		text := fmt.Sprintf("✏️ *Введите новую группу (0-10) для пользователя %s:*", escapeMarkdown(userID))
		editMsg := tgbotapi.NewEditMessageText(chatID, msgID, text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = getCancelInlineKeyboard()
		sentMsg, _ := bot.Send(editMsg)

		states, _ := loadStates()
		var stateInfo *StateInfo
		for i := range states {
			if states[i].ChatID == chatID {
				stateInfo = &states[i]
				break
			}
		}
		if stateInfo == nil {
			stateInfo = &StateInfo{ChatID: chatID, State: "edit_wait_group", EditUserID: userID, TempMsgID: sentMsg.MessageID, MainMsgID: msgID}
			states = append(states, *stateInfo)
		} else {
			stateInfo.State = "edit_wait_group"
			stateInfo.EditUserID = userID
			stateInfo.TempMsgID = sentMsg.MessageID
			stateInfo.MainMsgID = msgID
		}
		saveStates(states)
		return
	}

	if strings.HasPrefix(data, "edit:expires:") {
		userID := strings.TrimPrefix(data, "edit:expires:")
		text := fmt.Sprintf("✏️ *Введите новую дату (ДД.ММ.ГГГГ) для пользователя %s:*", escapeMarkdown(userID))
		editMsg := tgbotapi.NewEditMessageText(chatID, msgID, text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = getExpiresInlineKeyboard()
		sentMsg, _ := bot.Send(editMsg)

		states, _ := loadStates()
		var stateInfo *StateInfo
		for i := range states {
			if states[i].ChatID == chatID {
				stateInfo = &states[i]
				break
			}
		}
		if stateInfo == nil {
			stateInfo = &StateInfo{ChatID: chatID, State: "edit_wait_expires", EditUserID: userID, TempMsgID: sentMsg.MessageID, MainMsgID: msgID}
			states = append(states, *stateInfo)
		} else {
			stateInfo.State = "edit_wait_expires"
			stateInfo.EditUserID = userID
			stateInfo.TempMsgID = sentMsg.MessageID
			stateInfo.MainMsgID = msgID
		}
		saveStates(states)
		return
	}

	if strings.HasPrefix(data, "edit:comment:") {
		userID := strings.TrimPrefix(data, "edit:comment:")
		text := fmt.Sprintf("✏️ *Введите новый комментарий для пользователя %s:*", escapeMarkdown(userID))
		editMsg := tgbotapi.NewEditMessageText(chatID, msgID, text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = getCommentInlineKeyboard()
		sentMsg, _ := bot.Send(editMsg)

		states, _ := loadStates()
		var stateInfo *StateInfo
		for i := range states {
			if states[i].ChatID == chatID {
				stateInfo = &states[i]
				break
			}
		}
		if stateInfo == nil {
			stateInfo = &StateInfo{ChatID: chatID, State: "edit_wait_comment", EditUserID: userID, TempMsgID: sentMsg.MessageID, MainMsgID: msgID}
			states = append(states, *stateInfo)
		} else {
			stateInfo.State = "edit_wait_comment"
			stateInfo.EditUserID = userID
			stateInfo.TempMsgID = sentMsg.MessageID
			stateInfo.MainMsgID = msgID
		}
		saveStates(states)
		return
	}

	if strings.HasPrefix(data, "edit:adult:") {
		userID := strings.TrimPrefix(data, "edit:adult:")
		users, _ := loadUsers()
		for i := range users {
			if users[i].ID == userID {
				users[i].Params.Adult = !users[i].Params.Adult
				saveUsers(users)
				break
			}
		}
		handleUserDetail(chatID, bot, userID, msgID)
		return
	}

        if strings.HasPrefix(data, "edit:tv:") {
                userID := strings.TrimPrefix(data, "edit:tv:")
                users, _ := loadUsers()
                for i := range users {
                        if users[i].ID == userID {
                                 users[i].Params.Tv = !users[i].Params.Tv
                                 saveUsers(users)
                                 break
                        }
                }
                handleUserDetail(chatID, bot, userID, msgID)
                return
        }

	if strings.HasPrefix(data, "edit:admin:") {
		userID := strings.TrimPrefix(data, "edit:admin:")
		users, _ := loadUsers()
		for i := range users {
			if users[i].ID == userID {
				users[i].Params.Admin = !users[i].Params.Admin
				saveUsers(users)
				break
			}
		}
		handleUserDetail(chatID, bot, userID, msgID)
		return
	}
}

// ========== CREATE USER FLOW ==========
func handleNewUserStart(chatID int64, bot *tgbotapi.BotAPI, messageID int) {
	states, _ := loadStates()
	var stateInfo *StateInfo
	for i := range states {
		if states[i].ChatID == chatID {
			stateInfo = &states[i]
			break
		}
	}
	if stateInfo == nil {
		stateInfo = &StateInfo{ChatID: chatID, State: "newUser_id"}
		states = append(states, *stateInfo)
	} else {
		stateInfo.State = "newUser_id"
		stateInfo.TempUser = nil
	}
	saveStates(states)

	text := "📝 *Введите ID нового пользователя:*\n• Не менее 6 символов\n• Только цифры"
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = getCancelInlineKeyboard()
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

func showGroupInput(chatID int64, bot *tgbotapi.BotAPI, messageID int) {
	text := "🔢 *Введите группу (0-10)* или нажмите кнопку:"
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = getGroupInlineKeyboard()
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

func showExpiresInput(chatID int64, bot *tgbotapi.BotAPI, messageID int) {
	text := "📅 *Введите дату (ДД.ММ.ГГГГ)* или выберите срок:"
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = getExpiresInlineKeyboard()
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

func showCommentInput(chatID int64, bot *tgbotapi.BotAPI, messageID int) {
	text := "💬 *Введите комментарий* или нажмите 'Без комментария':"
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = getCommentInlineKeyboard()
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

// ========== DELETE USER ==========
func showDeleteConfirmation(chatID int64, bot *tgbotapi.BotAPI, messageID int, userID string) {
	text := fmt.Sprintf("⚠️ *Вы уверены, что хотите удалить пользователя %s?*\nЭто действие необратимо.", escapeMarkdown(userID))
	inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да, удалить", fmt.Sprintf("confirm_delete:%s", userID)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Нет, отмена", "cancel"),
		),
	)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = &inlineKeyboard
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

func confirmDeleteUser(chatID int64, bot *tgbotapi.BotAPI, messageID int, userID string) {
	users, _ := loadUsers()
	newUsers := []User{}
	for _, u := range users {
		if u.ID != userID {
			newUsers = append(newUsers, u)
		}
	}
	saveUsers(newUsers)

	err := db.RemoveUserFromDbByChatID(userID)
	if err != nil {
		log.Printf("⚠️ Ошибка удаления из data.db: %v", err)
	}

	text := fmt.Sprintf("✅ Пользователь %s удален", escapeMarkdown(userID))
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	backKb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Назад в меню", "back_to_menu"),
		),
	)
	editMsg.ReplyMarkup = &backKb
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

// ========== EDIT USER ==========
func startEditUser(chatID int64, bot *tgbotapi.BotAPI, messageID int, userID string) {
	text := fmt.Sprintf("✏️ *Редактирование пользователя %s*\n\nВыберите что изменить:", escapeMarkdown(userID))

	var sentMsg tgbotapi.Message
	if messageID != 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
		editMsg.ParseMode = "Markdown"
		editMsg.ReplyMarkup = getEditInlineKeyboard(userID)
		sentMsg, _ = bot.Send(editMsg)
	} else {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = getEditInlineKeyboard(userID)
		sentMsg, _ = bot.Send(msg)
	}

	states, _ := loadStates()
	var stateInfo *StateInfo
	for i := range states {
		if states[i].ChatID == chatID {
			stateInfo = &states[i]
			break
		}
	}
	if stateInfo == nil {
		stateInfo = &StateInfo{ChatID: chatID, State: "edit_menu", EditUserID: userID, MainMsgID: sentMsg.MessageID}
		states = append(states, *stateInfo)
	} else {
		stateInfo.State = "edit_menu"
		stateInfo.EditUserID = userID
		stateInfo.MainMsgID = sentMsg.MessageID
	}
	saveStates(states)
}

// ========== WHITELIST ==========
func handleAddWhitelistStart(chatID int64, bot *tgbotapi.BotAPI, messageID int) {
	text := "📝 *Введите Telegram ID пользователя для добавления в белый список:*\n\nПример: `123456789`"
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	editMsg.ReplyMarkup = getCancelInlineKeyboard()
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)

	states, _ := loadStates()
	var stateInfo *StateInfo
	for i := range states {
		if states[i].ChatID == chatID {
			stateInfo = &states[i]
			break
		}
	}
	if stateInfo == nil {
		stateInfo = &StateInfo{ChatID: chatID, State: "add_whitelist"}
		states = append(states, *stateInfo)
	} else {
		stateInfo.State = "add_whitelist"
	}
	saveStates(states)
}

func handleRemoveWhitelistStart(chatID int64, bot *tgbotapi.BotAPI, messageID int, callbackID string) {
	if len(allowedUserIDs) == 0 {
		callback := tgbotapi.NewCallback(callbackID, "📭 Белый список пуст")
		bot.Send(callback)
		return
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for id := range allowedUserIDs {
		if isAdmin(id) {
			continue
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("🗑 %d", id), fmt.Sprintf("whitelist_remove:%d", id)),
		))
	}
	if len(rows) == 0 {
		text := "📭 В белом списке только администратор, удалять некого"
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
		editMsg.ParseMode = "Markdown"
		backKb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("◀️ Назад в меню", "back_to_menu"),
			),
		)
		editMsg.ReplyMarkup = &backKb
		sentMsg, _ := bot.Send(editMsg)
		updateMainMsgID(chatID, sentMsg.MessageID)
		return
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Назад в меню", "back_to_menu"),
	))

	text := "🗑 *Выберите пользователя для удаления из белого списка:*"
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	editMsg.ParseMode = "Markdown"
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	editMsg.ReplyMarkup = &kb
	sentMsg, _ := bot.Send(editMsg)
	updateMainMsgID(chatID, sentMsg.MessageID)
}

// ========== MESSAGE HANDLERS ==========
func handleAdminMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, messageID int) {
	chatID := msg.Chat.ID
	text := msg.Text

	states, _ := loadStates()
	var currentState *StateInfo
	for i := range states {
		if states[i].ChatID == chatID {
			currentState = &states[i]
			break
		}
	}
	if currentState == nil {
		currentState = &StateInfo{ChatID: chatID, State: "start"}
		states = append(states, *currentState)
		saveStates(states)
	}

	if messageID == 0 {
		messageID = getMainMsgID(chatID)
	}

	switch currentState.State {
	case "newUser_id":
		userID := strings.ToLower(text)
		if !regexp.MustCompile(`^\d+$`).MatchString(userID) {
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "⚠️ ID должен содержать только цифры.")
			editMsg.ParseMode = "Markdown"
			editMsg.ReplyMarkup = getCancelInlineKeyboard()
			sentMsg, _ := bot.Send(editMsg)
			updateMainMsgID(chatID, sentMsg.MessageID)
			bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
			return
		}
		if len(userID) < 6 {
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "⚠️ ID должен быть не короче 6 символов")
			editMsg.ParseMode = "Markdown"
			editMsg.ReplyMarkup = getCancelInlineKeyboard()
			sentMsg, _ := bot.Send(editMsg)
			updateMainMsgID(chatID, sentMsg.MessageID)
			bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
			return
		}
		users, _ := loadUsers()
		for _, u := range users {
			if u.ID == userID {
				editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("⚠️ ID %s уже существует", userID))
				editMsg.ParseMode = "Markdown"
				editMsg.ReplyMarkup = getCancelInlineKeyboard()
				sentMsg, _ := bot.Send(editMsg)
				updateMainMsgID(chatID, sentMsg.MessageID)
				bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
				return
			}
		}

		// Удаляем сообщение пользователя с введенным ID
		bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))

		currentState.TempUser = &User{ID: userID}
		currentState.State = "newUser_group"
		saveStates(states)
		showGroupInput(chatID, bot, messageID)

	case "newUser_group":
		group, err := strconv.Atoi(text)
		if err != nil || group < 0 || group > 10 {
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "⚠️ Введите число от 0 до 10")
			editMsg.ParseMode = "Markdown"
			editMsg.ReplyMarkup = getGroupInlineKeyboard()
			sentMsg, _ := bot.Send(editMsg)
			updateMainMsgID(chatID, sentMsg.MessageID)
			bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
			return
		}

		// Удаляем сообщение пользователя с введенной группой
		bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))

		if currentState.TempUser == nil {
			currentState.TempUser = &User{}
		}
		currentState.TempUser.Group = group
		currentState.State = "newUser_expires"
		saveStates(states)
		showExpiresInput(chatID, bot, messageID)

	case "newUser_expires":
		expires, err := time.Parse("02.01.2006", text)
		if err != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "⚠️ Неверный формат. Используйте ДД.ММ.ГГГГ")
			editMsg.ParseMode = "Markdown"
			editMsg.ReplyMarkup = getExpiresInlineKeyboard()
			sentMsg, _ := bot.Send(editMsg)
			updateMainMsgID(chatID, sentMsg.MessageID)
			bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
			return
		}

		// Удаляем сообщение пользователя с введенной датой
		bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))

		currentState.TempUser.Expires = expires
		currentState.State = "newUser_comment"
		saveStates(states)
		showCommentInput(chatID, bot, messageID)

	case "newUser_comment":
		comment := text

		// Удаляем сообщение пользователя с комментарием
		bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))

		currentState.TempUser.Comment = comment
		users, _ := loadUsers()
		users = append(users, *currentState.TempUser)
		saveUsers(users)

		userIDint, err := strconv.ParseInt(currentState.TempUser.ID, 10, 64)
		var syncCode string
		if err == nil && userIDint > 0 {
			code, err := db.GenerateAndSaveCodeIntoDb(userIDint)
			if err == nil {
				syncCode = code
			}
		}

		resultText := fmt.Sprintf("✅ *Пользователь добавлен!*\n🆔 ID: `%s`\n🏷 Группа: `%d`\n📅 Доступ до: `%s`\n💬 Комментарий: `%s`\n🔞 Adult: false | 📺 TV: false | 👑 Admin: false\n🔑 Пароль: `%s`\n🔑 Код синхронизации: `%s`",
			escapeMarkdown(currentState.TempUser.ID), currentState.TempUser.Group,
			currentState.TempUser.Expires.Format("02.01.2006"), escapeMarkdown(comment),
			escapeMarkdown(currentState.TempUser.ID), syncCode)

                // Сохраняем ID пользователя
                userID := currentState.TempUser.ID

                // Очищаем состояние
		currentState.TempUser = nil
		currentState.State = "start"
		saveStates(states)

		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, resultText)
		editMsg.ParseMode = "Markdown"
                inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
                    tgbotapi.NewInlineKeyboardRow(
                        tgbotapi.NewInlineKeyboardButtonData("✏️ Редактировать", fmt.Sprintf("edit_user:%s", userID)),
                    ),
                    tgbotapi.NewInlineKeyboardRow(
                        tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("delete_user:%s", userID)),
                    ),
                    tgbotapi.NewInlineKeyboardRow(
                        tgbotapi.NewInlineKeyboardButtonData("◀️ Назад к списку", "back_to_user_list"),
                    ),
                )
		editMsg.ReplyMarkup = &inlineKeyboard
		sentMsg, _ := bot.Send(editMsg)
		updateMainMsgID(chatID, sentMsg.MessageID)

	case "add_whitelist":
		id, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ Неверный формат ID. Введите число.")
			editMsg.ParseMode = "Markdown"
			editMsg.ReplyMarkup = getCancelInlineKeyboard()
			sentMsg, _ := bot.Send(editMsg)
			updateMainMsgID(chatID, sentMsg.MessageID)
			bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
			return
		}

		if err := addToWhitelist(id); err != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ Ошибка сохранения: %v", err))
			editMsg.ParseMode = "Markdown"
			bot.Send(editMsg)
			bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
			return
		}

		bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))

		notifyMsg, _ := bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Пользователь %d добавлен в белый список", id)))

		time.AfterFunc(3*time.Second, func() {
			bot.Send(tgbotapi.NewDeleteMessage(chatID, notifyMsg.MessageID))
		})

		updateMainMenu(chatID, bot, messageID)

		currentState.State = "start"
		saveStates(states)
		return

	case "edit_wait_group":
		group, err := strconv.Atoi(text)
		if err != nil || group < 0 || group > 10 {
			editMsg := tgbotapi.NewEditMessageText(chatID, currentState.TempMsgID, "⚠️ Введите число от 0 до 10")
			editMsg.ParseMode = "Markdown"
			editMsg.ReplyMarkup = getCancelInlineKeyboard()
			sentMsg, _ := bot.Send(editMsg)
			updateMainMsgID(chatID, sentMsg.MessageID)
			bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
			return
		}

		bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))

		users, _ := loadUsers()
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				users[i].Group = group
				saveUsers(users)
				break
			}
		}

		handleUserDetail(chatID, bot, currentState.EditUserID, currentState.TempMsgID)

		currentState.State = "start"
		currentState.TempMsgID = 0
		saveStates(states)
		return

	case "edit_wait_expires":
		var expires time.Time
		if text == "📆 6 месяцев" {
			expires = time.Now().AddDate(0, 6, 0)
		} else if text == "📆 1 год" {
			expires = time.Now().AddDate(1, 0, 0)
		} else {
			var err error
			expires, err = time.Parse("02.01.2006", text)
			if err != nil {
				editMsg := tgbotapi.NewEditMessageText(chatID, currentState.TempMsgID, "⚠️ Неверный формат. Используйте ДД.ММ.ГГГГ")
				editMsg.ParseMode = "Markdown"
				editMsg.ReplyMarkup = getExpiresInlineKeyboard()
				sentMsg, _ := bot.Send(editMsg)
				updateMainMsgID(chatID, sentMsg.MessageID)
				bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))
				return
			}
		}

		bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))

		users, _ := loadUsers()
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				users[i].Expires = expires
				saveUsers(users)
				break
			}
		}

		handleUserDetail(chatID, bot, currentState.EditUserID, currentState.TempMsgID)

		currentState.State = "start"
		currentState.TempMsgID = 0
		saveStates(states)
		return

	case "edit_wait_comment":
		comment := text
		if comment == "💬 Без комментария" {
			comment = ""
		}

		bot.Send(tgbotapi.NewDeleteMessage(chatID, msg.MessageID))

		users, _ := loadUsers()
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				users[i].Comment = comment
				saveUsers(users)
				break
			}
		}

		handleUserDetail(chatID, bot, currentState.EditUserID, currentState.TempMsgID)

		currentState.State = "start"
		currentState.TempMsgID = 0
		saveStates(states)
		return

	default:
		updateMainMenu(chatID, bot, messageID)
	}
}

// ========== BOT BOOTSTRAP ==========
func BootstrapBot(appContext *AppContext) {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не найден в .env")
	}

	adminIDsStr := os.Getenv("TELEGRAM_ADMIN_ID")
	if adminIDsStr != "" {
		for _, idStr := range strings.Split(adminIDsStr, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
				adminIDs[id] = true
			}
		}
	}
	if len(adminIDs) == 0 {
		log.Println("⚠️ TELEGRAM_ADMIN_ID не задан, функции администратора отключены")
	}

	if err := loadWhitelist(); err != nil {
		log.Printf("⚠️ Ошибка загрузки белого списка: %v", err)
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = false
	log.Printf("Бот %s запущен", bot.Self.UserName)
	appContext.botName = bot.Self.UserName

	log.Printf("✅ Загружено %d разрешённых пользователей", len(allowedUserIDs))

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			go handleCallbackQuery(bot, update.CallbackQuery)
			continue
		}
		if update.Message == nil {
			continue
		}
		chatID := update.Message.Chat.ID
		telegramID := update.Message.From.ID

		// АДМИН
		if isAdmin(chatID) {
			log.Printf("🔧 Админ %d написал: %s", chatID, update.Message.Text)

			if update.Message.Text == "/start" {
				telegramIDStr := fmt.Sprintf("%d", telegramID)
				users, _ := loadUsers()
				existingUser := findUserByTelegramID(users, telegramID)
				if existingUser == nil {
					newUser := User{
						ID:      telegramIDStr,
						Group:   0,
						Expires: time.Now().AddDate(100, 0, 0),
						Comment: "Администратор",
						Params:  UserParams{Adult: false, Admin: false},
					}
					users = append(users, newUser)
					saveUsers(users)
				}
				code, err := db.GenerateAndSaveCodeIntoDb(chatID)
				if err != nil {
					log.Println("Ошибка сохранения кода:", err)
					bot.Send(tgbotapi.NewMessage(chatID, "Ошибка при генерации кода"))
					continue
				}
				response := fmt.Sprintf("👋 *Администратор*\n\n📌 *Ваш Telegram ID:* `%d`\n🔑 *Ваш пароль для входа:* `%d`\n🔑 *Ваш код для синхронизации:* `%s`\n\nВведите в настройках аккаунта Lampa.", telegramID, telegramID, code)
				msg := tgbotapi.NewMessage(chatID, response)
				msg.ParseMode = "Markdown"
				msg.ReplyMarkup = getMainMenuKeyboard()
				sentMsg, _ := bot.Send(msg)
				updateMainMsgID(chatID, sentMsg.MessageID)
				continue
			}

			msgID := getMainMsgID(chatID)
			if msgID == 0 {
				msg := tgbotapi.NewMessage(chatID, "👋 Главное меню")
				msg.ReplyMarkup = getMainMenuKeyboard()
				sentMsg, _ := bot.Send(msg)
				msgID = sentMsg.MessageID
				updateMainMsgID(chatID, msgID)
			}
			go handleAdminMessage(bot, update.Message, msgID)
			continue
		}

		// ОБЫЧНЫЙ ПОЛЬЗОВАТЕЛЬ
		if !isAllowed(chatID) {
			response := fmt.Sprintf("⛔ *Доступ запрещён.*\n\n📌 *Ваш Telegram ID:* `%d`\n\nСообщите этот ID администратору.", telegramID)
			msg := tgbotapi.NewMessage(chatID, response)
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			continue
		}
		if update.Message.Text == "/start" {
			users, _ := loadUsers()
			existingUser := findUserByTelegramID(users, telegramID)
			if existingUser == nil {
				_, err := autoCreateUser(telegramID)
				if err != nil {
					log.Println("Ошибка создания пользователя:", err)
					bot.Send(tgbotapi.NewMessage(chatID, "Ошибка при создании пользователя"))
					continue
				}
				users, _ = loadUsers()
				existingUser = findUserByTelegramID(users, telegramID)
			}
			code, err := db.GenerateAndSaveCodeIntoDb(chatID)
			if err != nil {
				log.Println("Ошибка сохранения кода:", err)
				bot.Send(tgbotapi.NewMessage(chatID, "Ошибка при генерации кода"))
				continue
			}
			password := generatePassword(telegramID)
			expiresDate := existingUser.Expires.Format("02.01.2006")
			response := fmt.Sprintf("🔑 *Ваш пароль для входа:* `%s`\n🔑 *Ваш код для синхронизации:* `%s`\n📅 *Ваша подписка активна до:* `%s`", password, code, expiresDate)
			msg := tgbotapi.NewMessage(chatID, response)
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			continue
		}
		msg := tgbotapi.NewMessage(chatID, "🤖 Используйте команду /start для получения токена синхронизации")
		bot.Send(msg)
	}
}
