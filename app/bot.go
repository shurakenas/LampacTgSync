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
	Admin bool `json:"admin"`
}

type StateInfo struct {
	ChatID      int64  `json:"chatId"`
	State       string `json:"state"`
	TempUser    *User  `json:"tempUser,omitempty"`
	EditUserID  string `json:"editUserId,omitempty"`
	EditAction  string `json:"editAction,omitempty"`
}

var adminIDs map[int64]bool = make(map[int64]bool)
var allowedUserIDs map[int64]bool = make(map[int64]bool)

func isAdmin(chatID int64) bool {
    return adminIDs[chatID]
}

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

func addToWhitelist(chatID int64) error {
	allowedUserIDs[chatID] = true
	return saveWhitelist()
}

func removeFromWhitelist(chatID int64) error {
	delete(allowedUserIDs, chatID)
	return saveWhitelist()
}

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
    // Экранируем только спецсимволы Markdown, но НЕ точки
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
        // "." , "\\",   // ТОЧКИ НЕ ЭКРАНИРУЕМ
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
		Params:  UserParams{Adult: false, Admin: false},
	}

	users = append(users, newUser)
	if err := saveUsers(users); err != nil {
		return "", err
	}

	log.Printf("✅ Автоматически создан пользователь %s", newUser.ID)
	return generatePassword(telegramID), nil
}

func getAdminKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🆕 Новый пользователь"),
			tgbotapi.NewKeyboardButton("👥 Пользователи"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🗑 Удалить пользователя"),
			tgbotapi.NewKeyboardButton("✏️ Редактировать пользователя"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("➕ Добавить в белый список"),
			tgbotapi.NewKeyboardButton("🗑 Удалить из белого списка"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📋 Белый список"),
		),
	)
}

func getCancelKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("❌ Отменить"),
		),
	)
}

func getGroupKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("1️⃣ Группа 1"),
			tgbotapi.NewKeyboardButton("2️⃣ Группа 2"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("❌ Отменить"),
		),
	)
}

func getExpiresKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📆 6 месяцев"),
			tgbotapi.NewKeyboardButton("📆 1 год"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("❌ Отменить"),
		),
	)
}

func getCommentKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("💬 Без комментария"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("❌ Отменить"),
		),
	)
}

func getEditKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🏷 Группа"),
			tgbotapi.NewKeyboardButton("📅 Срок"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("💬 Комментарий"),
			tgbotapi.NewKeyboardButton("🔞 Adult"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("👑 Admin"),
			tgbotapi.NewKeyboardButton("❌ Отменить"),
		),
	)
}

func handleCallbackQuery(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
	parts := strings.SplitN(callback.Data, ":", 2)
	if len(parts) != 2 {
		return
	}

	action := parts[0]
	userID := parts[1]

	switch action {
	case "whitelist_remove":
		id, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(callback.Message.Chat.ID, "❌ Ошибка: неверный ID"))
			return
		}

		if allowedUserIDs[id] {
			delete(allowedUserIDs, id)
			saveWhitelist()
			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, fmt.Sprintf("✅ Пользователь `%d` удалён из белого списка", id))
			msg.ParseMode = "Markdown"
			bot.Send(msg)
		} else {
			bot.Send(tgbotapi.NewMessage(callback.Message.Chat.ID, "⚠️ Пользователь не найден в белом списке"))
		}
		bot.Send(tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID))
		return
	}

	users, err := loadUsers()
	if err != nil {
		return
	}

	var userIndex int = -1
	for i, u := range users {
		if u.ID == userID {
			userIndex = i
			break
		}
	}

	if userIndex == -1 {
		bot.Send(tgbotapi.NewMessage(callback.Message.Chat.ID, "⚠️ Пользователь не найден"))
		return
	}

	switch action {
	case "delete":
		users = append(users[:userIndex], users[userIndex+1:]...)
		saveUsers(users)

                // Удаляем запись из data.db по chat_id (userID)
                err := db.RemoveUserFromDbByChatID(userID)
                if err != nil {
                    log.Printf("⚠️ Ошибка удаления из data.db: %v", err)
                }

		bot.Send(tgbotapi.NewMessage(callback.Message.Chat.ID, fmt.Sprintf("✅ Пользователь %s удален", escapeMarkdown(userID))))

	case "extend":
		users[userIndex].Expires = users[userIndex].Expires.AddDate(0, 1, 0)
		saveUsers(users)
		bot.Send(tgbotapi.NewMessage(callback.Message.Chat.ID, fmt.Sprintf(
			"✅ Доступ для %s продлен до %s",
			escapeMarkdown(userID), users[userIndex].Expires.Format("02.01.2006"),
		)))

	case "edit":
		states, _ := loadStates()
		var stateInfo *StateInfo
		for i := range states {
			if states[i].ChatID == callback.Message.Chat.ID {
				stateInfo = &states[i]
				break
			}
		}
		if stateInfo == nil {
			stateInfo = &StateInfo{ChatID: callback.Message.Chat.ID, State: "edit_menu"}
			states = append(states, *stateInfo)
		}
		stateInfo.State = "edit_menu"
		stateInfo.EditUserID = userID
		saveStates(states)

		user := &users[userIndex]
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, fmt.Sprintf(
			"✏️ *Редактирование пользователя %s*\n\n"+
				"🏷 Группа: %d\n"+
				"📅 Срок: %s\n"+
				"💬 Комментарий: %s\n"+
				"🔞 Adult: %v\n"+
				"👑 Admin: %v\n\n"+
				"Выберите что изменить:",
			escapeMarkdown(user.ID), user.Group, user.Expires.Format("02.01.2006"),
			escapeMarkdown(user.Comment), user.Params.Adult, user.Params.Admin,
		))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = getEditKeyboard()
		bot.Send(msg)
	}

	bot.Send(tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID))
}

func handleAdminMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
        log.Printf("📩 handleAdminMessage: %s от %d", msg.Text, chatID)

	states, _ := loadStates()
	var currentState *StateInfo
	for i := range states {
		if states[i].ChatID == chatID {
			currentState = &states[i]
                        log.Printf("📌 Текущее состояние: %s", currentState.State)
			break
		}
	}
	if currentState == nil {
		currentState = &StateInfo{ChatID: chatID, State: "start"}
		states = append(states, *currentState)
                log.Printf("📌 Создано новое состояние: start")
	}

	// Обработка редактирования группы
	if currentState.State == "edit_group" {
		if msg.Text == "❌ Отменить" {
			currentState.State = "edit_menu"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "✏️ Редактирование отменено")
			reply.ReplyMarkup = getEditKeyboard()
			bot.Send(reply)
			return
		}

		group, err := strconv.Atoi(msg.Text)
		if err != nil || group < 0 || group > 10 {
			reply := tgbotapi.NewMessage(chatID, "⚠️ Введите число от 0 до 10")
			reply.ReplyMarkup = getCancelKeyboard()
			bot.Send(reply)
			return
		}

		users, _ := loadUsers()
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				users[i].Group = group
				saveUsers(users)
				break
			}
		}

		currentState.State = "edit_menu"
		saveStates(states)

		users, _ = loadUsers()
		var user *User
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				user = &users[i]
				break
			}
		}

		reply := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"✅ Группа изменена на %d\n\n"+
				"🏷 Группа: %d\n📅 Срок: %s\n💬 Комментарий: %s\n🔞 Adult: %v\n👑 Admin: %v",
			group, user.Group, user.Expires.Format("02.01.2006"),
			user.Comment, user.Params.Adult, user.Params.Admin,
		))
		reply.ReplyMarkup = getEditKeyboard()
		bot.Send(reply)
		return
	}

	// Обработка редактирования срока
	if currentState.State == "edit_expires" {
		if msg.Text == "❌ Отменить" {
			currentState.State = "edit_menu"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "✏️ Редактирование отменено")
			reply.ReplyMarkup = getEditKeyboard()
			bot.Send(reply)
			return
		}

		var expires time.Time
		if msg.Text == "📆 6 месяцев" {
			expires = time.Now().AddDate(0, 6, 0)
		} else if msg.Text == "📆 1 год" {
			expires = time.Now().AddDate(1, 0, 0)
		} else {
			var err error
			expires, err = time.Parse("02.01.2006", msg.Text)
			if err != nil {
				reply := tgbotapi.NewMessage(chatID, "⚠️ Неверный формат. Используйте ДД.ММ.ГГГГ")
				reply.ReplyMarkup = getExpiresKeyboard()
				bot.Send(reply)
				return
			}
		}

		users, _ := loadUsers()
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				users[i].Expires = expires
				saveUsers(users)
				break
			}
		}

		currentState.State = "edit_menu"
		saveStates(states)

		users, _ = loadUsers()
		var user *User
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				user = &users[i]
				break
			}
		}

		reply := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"✅ Срок изменён на %s\n\n"+
				"🏷 Группа: %d\n📅 Срок: %s\n💬 Комментарий: %s\n🔞 Adult: %v\n👑 Admin: %v",
			expires.Format("02.01.2006"), user.Group, user.Expires.Format("02.01.2006"),
			user.Comment, user.Params.Adult, user.Params.Admin,
		))
		reply.ReplyMarkup = getEditKeyboard()
		bot.Send(reply)
		return
	}

	// Обработка редактирования комментария
	if currentState.State == "edit_comment" {
		if msg.Text == "❌ Отменить" {
			currentState.State = "edit_menu"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "✏️ Редактирование отменено")
			reply.ReplyMarkup = getEditKeyboard()
			bot.Send(reply)
			return
		}

		comment := msg.Text
		if comment == "💬 Без комментария" {
			comment = ""
		}

		users, _ := loadUsers()
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				users[i].Comment = comment
				saveUsers(users)
				break
			}
		}

		currentState.State = "edit_menu"
		saveStates(states)

		users, _ = loadUsers()
		var user *User
		for i := range users {
			if users[i].ID == currentState.EditUserID {
				user = &users[i]
				break
			}
		}

		reply := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"✅ Комментарий изменён: %s\n\n"+
				"🏷 Группа: %d\n📅 Срок: %s\n💬 Комментарий: %s\n🔞 Adult: %v\n👑 Admin: %v",
			comment, user.Group, user.Expires.Format("02.01.2006"),
			user.Comment, user.Params.Adult, user.Params.Admin,
		))
		reply.ReplyMarkup = getEditKeyboard()
		bot.Send(reply)
		return
	}

	// Обработка кнопок редактирования из меню
	if currentState.State == "edit_menu" {
		switch msg.Text {
		case "🏷 Группа":
			currentState.State = "edit_group"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "Введите новую группу (0-10):")
			reply.ReplyMarkup = getCancelKeyboard()
			bot.Send(reply)
			return
		case "📅 Срок":
			currentState.State = "edit_expires"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "Введите новую дату (ДД.ММ.ГГГГ) или выберите срок:")
			reply.ReplyMarkup = getExpiresKeyboard()
			bot.Send(reply)
			return
		case "💬 Комментарий":
			currentState.State = "edit_comment"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "Введите новый комментарий:")
			reply.ReplyMarkup = getCommentKeyboard()
			bot.Send(reply)
			return
		case "🔞 Adult":
			users, _ := loadUsers()
			for i := range users {
				if users[i].ID == currentState.EditUserID {
					users[i].Params.Adult = !users[i].Params.Adult
					saveUsers(users)
					break
				}
			}
			users, _ = loadUsers()
			var user *User
			for i := range users {
				if users[i].ID == currentState.EditUserID {
					user = &users[i]
					break
				}
			}
			reply := tgbotapi.NewMessage(chatID, fmt.Sprintf(
				"✅ Adult: %v\n\n"+
					"🏷 Группа: %d\n📅 Срок: %s\n💬 Комментарий: %s\n🔞 Adult: %v\n👑 Admin: %v",
				user.Params.Adult, user.Group, user.Expires.Format("02.01.2006"),
				user.Comment, user.Params.Adult, user.Params.Admin,
			))
			reply.ReplyMarkup = getEditKeyboard()
			bot.Send(reply)
			return
		case "👑 Admin":
			users, _ := loadUsers()
			for i := range users {
				if users[i].ID == currentState.EditUserID {
					users[i].Params.Admin = !users[i].Params.Admin
					saveUsers(users)
					break
				}
			}
			users, _ = loadUsers()
			var user *User
			for i := range users {
				if users[i].ID == currentState.EditUserID {
					user = &users[i]
					break
				}
			}
			reply := tgbotapi.NewMessage(chatID, fmt.Sprintf(
				"✅ Admin: %v\n\n"+
					"🏷 Группа: %d\n📅 Срок: %s\n💬 Комментарий: %s\n🔞 Adult: %v\n👑 Admin: %v",
				user.Params.Admin, user.Group, user.Expires.Format("02.01.2006"),
				user.Comment, user.Params.Adult, user.Params.Admin,
			))
			reply.ReplyMarkup = getEditKeyboard()
			bot.Send(reply)
			return
		case "❌ Отменить":
			currentState.State = "start"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "✅ Редактирование завершено")
			reply.ReplyMarkup = getAdminKeyboard()
			bot.Send(reply)
			return
		}
	}

	// Обычные админ-команды
	switch msg.Text {
	case "🆕 Новый пользователь":
                log.Printf("🔧 Обработка НОВЫЙ ПОЛЬЗОВАТЕЛЬ")
		currentState.State = "newUser_id"
		currentState.TempUser = nil
		currentState.EditUserID = ""
                if err := saveStates(states); err != nil {
                    log.Printf("❌ Ошибка сохранения состояния: %v", err)
                }
		response := "📝 Введите ID нового пользователя:\n• Не менее 6 символов\n• Только цифры"
		reply := tgbotapi.NewMessage(chatID, response)
//		reply.ParseMode = "Markdown"
		reply.ReplyMarkup = getCancelKeyboard()
                if _, err := bot.Send(reply); err != nil {
                    log.Printf("❌ Ошибка отправки сообщения: %v", err)
                }
                log.Printf("✅ Состояние изменено на newUser_id, ответ отправлен")
                return

	case "👥 Пользователи":
		handleListUsers(chatID, bot)

	case "🗑 Удалить пользователя":
		handleDeleteUser(chatID, bot)

	case "✏️ Редактировать пользователя":
		handleEditUserSelect(chatID, bot)

	case "➕ Добавить в белый список":
		currentState.State = "add_whitelist"
		currentState.TempUser = nil
		saveStates(states)
		reply := tgbotapi.NewMessage(chatID, "📝 *Введите Telegram ID пользователя для добавления в белый список:*")
		reply.ParseMode = "Markdown"
		reply.ReplyMarkup = getCancelKeyboard()
		bot.Send(reply)

	case "🗑 Удалить из белого списка":
		handleRemoveFromWhitelist(chatID, bot)

	case "📋 Белый список":
		handleShowWhitelist(chatID, bot)

	default:
		// Создание нового пользователя
		switch currentState.State {
		case "newUser_id":
			if msg.Text == "❌ Отменить" {
				currentState.State = "start"
				saveStates(states)
				reply := tgbotapi.NewMessage(chatID, "🚫 Добавление отменено")
				reply.ReplyMarkup = getAdminKeyboard()
				bot.Send(reply)
				return
			}

			userID := strings.ToLower(msg.Text)

                        // Проверяем, что введены только цифры
                        if !regexp.MustCompile(`^\d+$`).MatchString(userID) {
                            reply := tgbotapi.NewMessage(chatID, "⚠️ ID должен содержать только цифры.")
                            reply.ReplyMarkup = getCancelKeyboard()
                            bot.Send(reply)
                            return
                        }

                        // Проверяем длину (Telegram ID обычно 9-10 цифр, но пусть будет не менее 6)
			if len(userID) < 6 {
				reply := tgbotapi.NewMessage(chatID, "⚠️ ID должен быть не короче 6 символов")
				reply.ReplyMarkup = getCancelKeyboard()
				bot.Send(reply)
				return
			}

			users, _ := loadUsers()
			for _, u := range users {
				if u.ID == userID {
					reply := tgbotapi.NewMessage(chatID, "⚠️ Этот ID уже существует")
					reply.ReplyMarkup = getCancelKeyboard()
					bot.Send(reply)
					return
				}
			}

			currentState.TempUser = &User{ID: userID}
			currentState.State = "newUser_group"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "🔢 *Введите группу (0-10)* или нажмите '1️⃣ Группа 1':")
			reply.ParseMode = "Markdown"
			reply.ReplyMarkup = getGroupKeyboard()
			bot.Send(reply)

		case "newUser_group":
			if msg.Text == "❌ Отменить" {
				currentState.State = "start"
				saveStates(states)
				reply := tgbotapi.NewMessage(chatID, "🚫 Добавление отменено")
				reply.ReplyMarkup = getAdminKeyboard()
				bot.Send(reply)
				return
			}

			var group int
			if msg.Text == "1️⃣ Группа 1" {
				group = 1
			} else if msg.Text == "2️⃣ Группа 2" {
				group = 2
			} else {
				g, err := strconv.Atoi(msg.Text)
				if err != nil || g < 0 || g > 10 {
					reply := tgbotapi.NewMessage(chatID, "⚠️ Введите число от 0 до 10")
					reply.ReplyMarkup = getGroupKeyboard()
					bot.Send(reply)
					return
				}
				group = g
			}

			currentState.TempUser.Group = group
			currentState.State = "newUser_expires"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "📅 *Введите дату (ДД.ММ.ГГГГ)* или выберите срок:")
			reply.ParseMode = "Markdown"
			reply.ReplyMarkup = getExpiresKeyboard()
			bot.Send(reply)

		case "newUser_expires":
			if msg.Text == "❌ Отменить" {
				currentState.State = "start"
				saveStates(states)
				reply := tgbotapi.NewMessage(chatID, "🚫 Добавление отменено")
				reply.ReplyMarkup = getAdminKeyboard()
				bot.Send(reply)
				return
			}

			var expires time.Time
			if msg.Text == "📆 6 месяцев" {
				expires = time.Now().AddDate(0, 6, 0)
			} else if msg.Text == "📆 1 год" {
				expires = time.Now().AddDate(1, 0, 0)
			} else {
				var err error
				expires, err = time.Parse("02.01.2006", msg.Text)
				if err != nil {
					reply := tgbotapi.NewMessage(chatID, "⚠️ Неверный формат. Используйте ДД.ММ.ГГГГ")
					reply.ReplyMarkup = getExpiresKeyboard()
					bot.Send(reply)
					return
				}
			}

			currentState.TempUser.Expires = expires
			currentState.State = "newUser_comment"
			saveStates(states)
			reply := tgbotapi.NewMessage(chatID, "💬 *Комментарий* или '💬 Без комментария':")
			reply.ParseMode = "Markdown"
			reply.ReplyMarkup = getCommentKeyboard()
			bot.Send(reply)

		case "newUser_comment":
			if msg.Text == "❌ Отменить" {
				currentState.State = "start"
				saveStates(states)
				reply := tgbotapi.NewMessage(chatID, "🚫 Добавление отменено")
				reply.ReplyMarkup = getAdminKeyboard()
				bot.Send(reply)
				return
			}

			if msg.Text != "💬 Без комментария" {
				currentState.TempUser.Comment = msg.Text
			}

			users, _ := loadUsers()
			users = append(users, *currentState.TempUser)
			saveUsers(users)

                        // Генерируем код синхронизации для нового пользователя
                        userIDStr := currentState.TempUser.ID
                        userIDint, err := strconv.ParseInt(userIDStr, 10, 64)
                        var syncCode string
                        if err == nil && userIDint > 0 {
                            code, err := db.GenerateAndSaveCodeIntoDb(userIDint)
                            if err == nil {
                                syncCode = code
                            } else {
                                syncCode = "❌ ошибка"
                            }
                        } else {
                            syncCode = "❌ ID не число"
                        }

			newUser := currentState.TempUser
			currentState.TempUser = nil
			currentState.State = "start"
			saveStates(states)

			response := fmt.Sprintf("✅ *Пользователь добавлен!*\n🆔 ID: %s\n🏷 Группа: %d\n📅 Доступ до: %s\n💬 Комментарий: %s\n🔑 Пароль для входа: `%s`\n🔑 Код синхронизации: `%s`",
				escapeMarkdown(newUser.ID), newUser.Group, newUser.Expires.Format("02.01.2006"), escapeMarkdown(newUser.Comment), escapeMarkdown(newUser.ID), syncCode)
			reply := tgbotapi.NewMessage(chatID, response)
			reply.ParseMode = "Markdown"
			reply.ReplyMarkup = getAdminKeyboard()
			bot.Send(reply)

		case "add_whitelist":
			if msg.Text == "❌ Отменить" {
				currentState.State = "start"
				saveStates(states)
				reply := tgbotapi.NewMessage(chatID, "🚫 Добавление в белый список отменено")
				reply.ReplyMarkup = getAdminKeyboard()
				bot.Send(reply)
				return
			}

			id, err := strconv.ParseInt(strings.TrimSpace(msg.Text), 10, 64)
			if err != nil {
				reply := tgbotapi.NewMessage(chatID, "❌ Неверный формат ID. Введите число.")
				reply.ReplyMarkup = getCancelKeyboard()
				bot.Send(reply)
				return
			}

			if err := addToWhitelist(id); err != nil {
				reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err))
				bot.Send(reply)
				return
			}

			currentState.State = "start"
			saveStates(states)

			reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Пользователь `%d` добавлен в белый список!", id))
			reply.ParseMode = "Markdown"
			reply.ReplyMarkup = getAdminKeyboard()
			bot.Send(reply)
		}
	}
}

func handleRemoveFromWhitelist(chatID int64, bot *tgbotapi.BotAPI) {
	if len(allowedUserIDs) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📭 Белый список пуст")
		bot.Send(msg)
		return
	}

	hasNonAdmin := false
	for id := range allowedUserIDs {
		if !isAdmin(id) {
			hasNonAdmin = true
			break
		}
	}

	if !hasNonAdmin {
		msg := tgbotapi.NewMessage(chatID, "📭 В белом списке только администратор, удалять некого")
		bot.Send(msg)
		return
	}

	for id := range allowedUserIDs {
		if isAdmin(id) {
			continue
		}
		inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить из белого списка", fmt.Sprintf("whitelist_remove:%d", id)),
			),
		)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("👤 Telegram ID: `%d`", id))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = inlineKeyboard
		bot.Send(msg)
	}
}

func handleEditUserSelect(chatID int64, bot *tgbotapi.BotAPI) {
	users, err := loadUsers()
	if err != nil || len(users) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📭 Список пользователей пуст")
		bot.Send(msg)
		return
	}

	for _, user := range users {
		inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✏️ Редактировать", fmt.Sprintf("edit:%s", user.ID)),
			),
		)
		expiresDate := user.Expires.Format("02.01.2006")
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"👤 *%s*\n🏷 Группа: %d\n📅 Доступ до: %s\n💬 Комментарий: %s",
			escapeMarkdown(user.ID), user.Group, expiresDate, escapeMarkdown(user.Comment),
		))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = inlineKeyboard
		bot.Send(msg)
	}
}

func handleDeleteUser(chatID int64, bot *tgbotapi.BotAPI) {
	users, err := loadUsers()
	if err != nil || len(users) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📭 Список пользователей пуст")
		bot.Send(msg)
		return
	}

	for _, user := range users {
		inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("delete:%s", user.ID)),
			),
		)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("👤 *%s*", escapeMarkdown(user.ID)))
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = inlineKeyboard
		bot.Send(msg)
	}
}

func handleListUsers(chatID int64, bot *tgbotapi.BotAPI) {
	users, err := loadUsers()
	if err != nil || len(users) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📭 Список пользователей пуст")
		bot.Send(msg)
		return
	}

	for _, user := range users {
		expiresDate := user.Expires.Format("02.01.2006")

                // Получаем код синхронизации
                userTelegramID, _ := strconv.ParseInt(user.ID, 10, 64)
                syncCode := "❌ нет"
                if userTelegramID > 0 {
                    code, err := db.GenerateAndSaveCodeIntoDb(userTelegramID)
                    if err == nil && code != "" {
                        syncCode = code
                    }
                }

		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"👤 %s\n🏷 Группа: %d\n📅 Доступ до: %s\n💬 Комментарий: %s\n🔞 Adult: %v | 👑 Admin: %v\n🔑 Пароль для входа: `%s`\n🔑 Код синхронизации: `%s`",
			escapeMarkdown(user.ID), user.Group, expiresDate, escapeMarkdown(user.Comment), user.Params.Adult, user.Params.Admin, escapeMarkdown(user.ID), syncCode,
		))
		msg.ParseMode = "Markdown"
		bot.Send(msg)
	}
}

func handleShowWhitelist(chatID int64, bot *tgbotapi.BotAPI) {
	if len(allowedUserIDs) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📭 Белый список пуст")
		bot.Send(msg)
		return
	}

	var ids []string
	for id := range allowedUserIDs {
		ids = append(ids, fmt.Sprintf("- `%d`", id))
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("📋 *Белый список Telegram ID:*\n%s", strings.Join(ids, "\n")))
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

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
                                // === ДОБАВЛЯЕМ АДМИНА В users.JSON ЕСЛИ ЕГО ТАМ НЕТ ===
                                telegramIDStr := fmt.Sprintf("%d", telegramID)
                                users, _ := loadUsers()
                                existingUser := findUserByTelegramID(users, telegramID)

                                if existingUser == nil {
                                    newUser := User{
                                        ID:      telegramIDStr,
                                        Group:   0,
                                        Expires: time.Now().AddDate(100, 0, 0), // На 100 лет
                                        Comment: "Администратор",
                                        Params:  UserParams{Adult: false, Admin: false},
                                    }
                                    users = append(users, newUser)
                                    saveUsers(users)
                                }

                                // Генерируем код синхронизации
				code, err := db.GenerateAndSaveCodeIntoDb(chatID)
				if err != nil {
					log.Println("Ошибка сохранения кода:", err)
					bot.Send(tgbotapi.NewMessage(chatID, "Ошибка при генерации кода"))
					continue
				}

				response := fmt.Sprintf(
					"👋 *Администратор*\n\n📌 *Ваш Telegram ID:* `%d`\n🔑 *Ваш пароль для входа:* `%d`\n🔑 *Ваш код для синхронизации:* `%s`\n\nВведите в настройках аккаунта Lampa.",
					telegramID, telegramID, code,
				)
				msg := tgbotapi.NewMessage(chatID, response)
				msg.ParseMode = "Markdown"
				msg.ReplyMarkup = getAdminKeyboard()
				bot.Send(msg)
				continue
			}

                        log.Printf("🔍 Отправляем в handleAdminMessage: %s", update.Message.Text)
			go handleAdminMessage(bot, update.Message)
			continue
		}

		// ОБЫЧНЫЙ ПОЛЬЗОВАТЕЛЬ
		if !isAllowed(chatID) {
			response := fmt.Sprintf(
				"⛔ *Доступ запрещён.*\n\n📌 *Ваш Telegram ID:* `%d`\n\nСообщите этот ID администратору или покиньте бота, если не знаете, зачем вы здесь.",
				telegramID,
			)
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

			response := fmt.Sprintf(
				"🔑 *Ваш пароль для входа:* `%s`\n🔑 *Ваш код для синхронизации:* `%s`\n📅 *Ваша подписка активна до:* `%s`",
				password, code, expiresDate,
			)
			msg := tgbotapi.NewMessage(chatID, response)
			msg.ParseMode = "Markdown"
			bot.Send(msg)
			continue
		}

		msg := tgbotapi.NewMessage(chatID, "🤖 Используйте команду /start для получения токена синхронизации")
		bot.Send(msg)
	}
}
