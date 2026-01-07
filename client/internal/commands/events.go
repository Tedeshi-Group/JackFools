package commands // Пакет commands содержит реализацию CLI команд.

import ( // Начинаем блок импортов.
	"context"       // Отменяем запросы при загрузке базы ответов.
	"encoding/json" // Парсим JSON сообщения и базу ответов.
	"fmt"           // Форматируем ошибки.
	"io"            // Читаем тело ответа HTTP.
	"log"           // Логируем события.
	"net/http"      // Отправляем HTTP запросы для загрузки базы ответов.
	"regexp"        // Используем регулярные выражения для удаления тегов форматирования.
	"strings"       // Работаем со строками для обработки тегов игр.
	"time"          // Устанавливаем таймауты для запросов.
) // Закрываем блок импортов.

// WebSocketMessage представляет базовое сообщение от WebSocket сервера.
// Формат сообщений от Jackbox: { "pc": number, "opcode": "string", "result": {...} }
type WebSocketMessage struct { // Структура базового сообщения.
	PC      int                    `json:"pc"`      // Порядковый номер сообщения (опционально).
	Opcode  string                 `json:"opcode"`  // Код операции (например, "client/welcome", "client/state").
	Result  map[string]interface{} `json:"result"`  // Результат операции (содержит entities и другую информацию).
	Type    string                 `json:"type"`    // Тип сообщения (для обратной совместимости, если используется старый формат).
	ID      string                 `json:"id"`      // ID сообщения (опционально, для обратной совместимости).
	Payload map[string]interface{} `json:"payload"` // Полезная нагрузка сообщения (для обратной совместимости).
} // Конец WebSocketMessage.

// GameEvent представляет типизированное событие игры.
type GameEvent struct { // Структура события игры.
	Type           string                 // Тип события (например, "question", "answer_choice").
	EventID        string                 // Уникальный ID события.
	GameTag        string                 // Тег игры (например, "quiplash2").
	Payload        map[string]interface{} // Полезная нагрузка события.
	RequiresAnswer bool                   // Требуется ли ответ на это событие.
} // Конец GameEvent.

// AnswerDatabase представляет базу правильных ответов.
// Поддерживает два формата:
// 1. Стандартный: { "games": { "gameTag": { "eventTypes": { "eventType": { "questionId": "answer" } } } } }
// 2. Прямой для triviadeath2: { "question": "answer" } или { "question": index }
// 3. Формат с content массивом: { "content": [ { "text": "question", "choices": [ { "correct": true/false, "text": "answer" } ] } ] }
type AnswerDatabase struct { // Структура базы ответов.
	Games map[string]GameAnswers `json:"games"` // Карта игр с их ответами (стандартный формат).
	// Для triviadeath2 также поддерживается прямой формат вопрос->ответ.
	Questions map[string]interface{} `json:"-"` // Прямые вопросы (парсим как map[string]interface{} для гибкости).
	// Для финального раунда: вопрос -> массив текстов правильных ответов (несколько правильных ответов).
	FinalRoundQuestions map[string][]string `json:"-"` // Вопросы финального раунда с текстами правильных ответов.
} // Конец AnswerDatabase.

// TriviaDeath2QuestionItem представляет элемент вопроса из формата с content массивом.
type TriviaDeath2QuestionItem struct { // Структура элемента вопроса.
	Text    string               `json:"text"`    // Текст вопроса.
	ID      string               `json:"id"`      // ID вопроса.
	Choices []TriviaDeath2Choice `json:"choices"` // Варианты ответов.
} // Конец TriviaDeath2QuestionItem.

// TriviaDeath2Choice представляет вариант ответа.
type TriviaDeath2Choice struct { // Структура варианта ответа.
	Text    string `json:"text"`    // Текст варианта ответа.
	Correct bool   `json:"correct"` // Является ли вариант правильным.
} // Конец TriviaDeath2Choice.

// TriviaDeath2ContentFormat представляет формат файла с content массивом.
type TriviaDeath2ContentFormat struct { // Структура формата с content.
	Content []TriviaDeath2QuestionItem `json:"content"` // Массив вопросов.
} // Конец TriviaDeath2ContentFormat.

// GameAnswers представляет ответы для конкретной игры.
type GameAnswers struct { // Структура ответов игры.
	EventTypes map[string]EventAnswers `json:"eventTypes"` // Карта типов событий с ответами.
} // Конец GameAnswers.

// EventAnswers представляет ответы для конкретного типа события.
type EventAnswers struct { // Структура ответов события.
	Answers map[string]string `json:"answers"` // Карта ID вопроса -> ответ.
} // Конец EventAnswers.

// parseWebSocketMessage парсит JSON сообщение от WebSocket сервера.
// Принимает сырые байты сообщения.
// Возвращает распарсенное сообщение или ошибку.
func parseWebSocketMessage(data []byte) (*WebSocketMessage, error) { // Функция парсинга сообщения.
	var msg WebSocketMessage                           // Создаём переменную для сообщения.
	if err := json.Unmarshal(data, &msg); err != nil { // Пытаемся распарсить JSON.
		return nil, fmt.Errorf("failed to unmarshal message: %w", err) // Возвращаем ошибку.
	} // Конец проверки парсинга.

	return &msg, nil // Возвращаем распарсенное сообщение.
} // Конец parseWebSocketMessage.

// parseGameEvent преобразует WebSocketMessage в GameEvent.
// Извлекает информацию о типе события, игре и необходимости ответа.
// Принимает WebSocket сообщение.
// Возвращает GameEvent или ошибку.
func parseGameEvent(msg *WebSocketMessage) (*GameEvent, error) { // Функция преобразования в GameEvent.
	if msg == nil { // Если сообщение nil.
		return nil, fmt.Errorf("message is nil") // Возвращаем ошибку.
	} // Конец проверки сообщения.

	event := &GameEvent{ // Создаём событие.
		Type:    msg.Opcode, // Устанавливаем тип из opcode (например, "client/welcome", "client/state").
		EventID: msg.ID,     // Устанавливаем ID из сообщения (если есть).
	} // Конец создания события.

	// Определяем payload в зависимости от формата сообщения.
	if msg.Result != nil { // Если используется новый формат с result.
		event.Payload = msg.Result // Используем result как payload.
	} else if msg.Payload != nil { // Если используется старый формат с payload.
		event.Payload = msg.Payload // Используем payload.
	} else { // Если ни result, ни payload нет.
		event.Payload = make(map[string]interface{}) // Создаём пустой payload.
	} // Конец проверки формата.

	// Извлекаем тег игры из result/entities/room или payload.
	if event.Payload != nil { // Если payload не nil.
		// Пытаемся извлечь из entities.room.analytics (новый формат).
		if entities, ok := event.Payload["entities"].(map[string]interface{}); ok { // Если есть entities.
			if room, ok := entities["room"].([]interface{}); ok && len(room) > 1 { // Если есть room в entities.
				if roomData, ok := room[1].(map[string]interface{}); ok { // Если roomData - это map.
					if roomVal, ok := roomData["val"].(map[string]interface{}); ok { // Если есть val.
						if analytics, ok := roomVal["analytics"].([]interface{}); ok && len(analytics) > 0 { // Если есть analytics.
							// Пробуем все элементы analytics, так как структура может быть разной.
							for _, analyticsItem := range analytics { // Проходим по каждому элементу analytics.
								if item, ok := analyticsItem.(map[string]interface{}); ok { // Если элемент - это map.
									if appid, ok := item["appid"].(string); ok && appid != "" { // Если есть appid и он не пустой.
										// Извлекаем тег игры из appid (например, "triviadeath2-tjsp-Win" -> "triviadeath2-tjsp").
										parts := strings.Split(appid, "-") // Разбиваем по дефису.
										if len(parts) >= 2 {               // Если есть хотя бы 2 части.
											event.GameTag = strings.Join(parts[:len(parts)-1], "-") // Берём все части кроме последней (убираем "-Win", "-Mac" и т.д.).
											break                                                   // Прерываем цикл, так как нашли тег.
										} else { // Если частей меньше 2.
											event.GameTag = appid // Используем весь appid.
											break                 // Прерываем цикл.
										} // Конец проверки частей.
									} // Конец проверки appid.
								} // Конец проверки item.
							} // Конец цикла по analytics.
						} // Конец проверки analytics.
					} // Конец проверки val.
				} // Конец проверки roomData.
			} // Конец проверки room.
		} // Конец проверки entities.

		// Если тег игры не найден, пытаемся извлечь из других полей (обратная совместимость).
		if event.GameTag == "" { // Если тег игры ещё не установлен.
			if gameTag, ok := event.Payload["gameTag"].(string); ok { // Если тег игры есть в payload.
				event.GameTag = gameTag // Устанавливаем тег игры.
			} else if appTag, ok := event.Payload["appTag"].(string); ok { // Если тег приложения есть в payload.
				event.GameTag = appTag // Устанавливаем тег игры из appTag.
			} // Конец проверки тега игры.
		} // Конец проверки установки тега игры.
	} // Конец проверки payload.

	// Определяем, требует ли событие ответа.
	// Это зависит от типа события и игры.
	event.RequiresAnswer = shouldRequireAnswer(event) // Определяем необходимость ответа.

	return event, nil // Возвращаем событие.
} // Конец parseGameEvent.

// shouldRequireAnswer определяет, требует ли событие ответа.
// Принимает GameEvent.
// Возвращает true, если событие требует ответа.
func shouldRequireAnswer(event *GameEvent) bool { // Функция определения необходимости ответа.
	if event == nil { // Если событие nil.
		return false // Не требует ответа.
	} // Конец проверки события.

	// Для triviadeath2-tjsp проверяем наличие audiencePlayer с hasSubmit: false.
	// Проверяем gameTag или если он пустой, но тип события "object" - тоже проверяем (gameTag может быть кеширован).
	if event.GameTag == "triviadeath2-tjsp" || strings.Contains(event.GameTag, "triviadeath2") || event.Type == "object" { // Если это Trivia Death 2 или событие типа "object".
		if event.Payload != nil { // Если payload не nil.
			// Формат 1: opcode "object" с result.key == "audiencePlayer" и result.val.
			if event.Type == "object" { // Если opcode = "object".
				if key, ok := event.Payload["key"].(string); ok && key == "audiencePlayer" { // Если key = "audiencePlayer".
					if val, ok := event.Payload["val"].(map[string]interface{}); ok { // Если есть val.
						// Проверяем roundType для финального раунда.
						roundType := ""                              // Переменная для типа раунда.
						if rt, ok := val["roundType"].(string); ok { // Если есть roundType.
							roundType = rt // Устанавливаем тип раунда.
						} // Конец проверки roundType.

						// Для финального раунда игнорируем hasSubmit, так как он может быть true даже когда вопрос активен.
						// Для обычного раунда проверяем hasSubmit.
						shouldCheckHasSubmit := roundType != "FinalRound" // Проверяем hasSubmit только для не-финального раунда.
						hasSubmitOk := true                               // По умолчанию считаем, что hasSubmit в порядке.
						if shouldCheckHasSubmit {                         // Если нужно проверить hasSubmit.
							if hasSubmit, ok := val["hasSubmit"].(bool); ok { // Если есть hasSubmit.
								hasSubmitOk = !hasSubmit // hasSubmit должен быть false (вопрос активен).
							} // Конец проверки hasSubmit.
						} // Конец проверки необходимости проверки hasSubmit.

						if hasSubmitOk { // Если hasSubmit в порядке (false для обычного раунда или игнорируется для финального).
							// Проверяем, есть ли choices (варианты ответов).
							if choices, ok := val["choices"].([]interface{}); ok && len(choices) > 0 { // Если есть варианты ответов.
								// Проверяем, есть ли prompt (текст вопроса).
								if prompt, ok := val["prompt"].(string); ok && prompt != "" { // Если есть prompt.
									return true // Требует ответа.
								} // Конец проверки prompt.
							} // Конец проверки choices.
						} // Конец проверки hasSubmit.
					} // Конец проверки val.
				} // Конец проверки key.
			} // Конец проверки opcode "object".

			// Формат 2: entities.audiencePlayer[1].val (старый формат).
			if entities, ok := event.Payload["entities"].(map[string]interface{}); ok { // Если есть entities.
				if audiencePlayer, ok := entities["audiencePlayer"].([]interface{}); ok && len(audiencePlayer) > 1 { // Если есть audiencePlayer.
					if playerData, ok := audiencePlayer[1].(map[string]interface{}); ok { // Если playerData - это map.
						if playerVal, ok := playerData["val"].(map[string]interface{}); ok { // Если есть val.
							if hasSubmit, ok := playerVal["hasSubmit"].(bool); ok && !hasSubmit { // Если hasSubmit = false (вопрос активен).
								// Проверяем, есть ли choices (варианты ответов).
								if choices, ok := playerVal["choices"].([]interface{}); ok && len(choices) > 0 { // Если есть варианты ответов.
									return true // Требует ответа.
								} // Конец проверки choices.
							} // Конец проверки hasSubmit.
						} // Конец проверки val.
					} // Конец проверки playerData.
				} // Конец проверки audiencePlayer.
			} // Конец проверки entities.
		} // Конец проверки payload.
	} // Конец проверки triviadeath2.

	// Для игры "everyday" проверяем opcode "audience/g-counter" или "client/welcome" с счетчиками.
	if event.GameTag == "everyday" { // Если это игра "everyday".
		if event.Type == "audience/g-counter" { // Если это событие "audience/g-counter".
			if event.Payload != nil { // Если payload не nil.
				if key, ok := event.Payload["key"].(string); ok && key != "" { // Если есть key и он не пустой.
					return true // Требует ответа.
				} // Конец проверки key.
			} // Конец проверки payload.
		} else if event.Type == "client/welcome" { // Если это событие "client/welcome".
			// Проверяем, есть ли в entities счетчики типа cat_count_<NUMBER>.
			if event.Payload != nil { // Если payload не nil.
				// Проверяем, есть ли result в payload (может быть напрямую в payload или в result).
				var entities map[string]interface{} // Переменная для entities.
				var foundEntities bool              // Флаг наличия entities.

				// Вариант 1: entities напрямую в payload.
				if entitiesData, ok := event.Payload["entities"].(map[string]interface{}); ok { // Если entities есть напрямую в payload.
					entities = entitiesData // Используем entities из payload.
					foundEntities = true    // Устанавливаем флаг.
				} else if result, ok := event.Payload["result"].(map[string]interface{}); ok { // Если есть result.
					if entitiesData, ok := result["entities"].(map[string]interface{}); ok { // Если есть entities в result.
						entities = entitiesData // Используем entities из result.
						foundEntities = true    // Устанавливаем флаг.
					} // Конец проверки entities в result.
				} // Конец проверки result.

				if foundEntities { // Если entities найдены.
					for entityKey := range entities { // Проходим по каждому ключу entity.
						// Проверяем, является ли ключ счетчиком типа cat_count_<NUMBER>.
						if strings.HasPrefix(entityKey, "cat_count_") { // Если ключ начинается с "cat_count_".
							log.Printf("coordinator: Everyday client/welcome found counter: %s", entityKey) // Логируем найденный счетчик.
							return true                                                                     // Требует ответа (найден счетчик).
						} // Конец проверки префикса.
					} // Конец цикла по entities.
					log.Printf("coordinator: Everyday client/welcome has entities but no cat_count_* counters found") // Логируем отсутствие счетчиков.
				} else { // Если entities не найдены.
					log.Printf("coordinator: Everyday client/welcome has no entities in payload") // Логируем отсутствие entities.
				} // Конец проверки наличия entities.
			} else { // Если payload nil.
				log.Printf("coordinator: Everyday client/welcome has nil payload") // Логируем отсутствие payload.
			} // Конец проверки payload.
		} // Конец проверки типа события.
	} // Конец проверки everyday.

	// Проверяем тип события (для других игр).
	// Для Quiplash 2 события выбора ответа обычно имеют тип "answer_choice" или "question".
	switch event.Type { // Проверяем тип события.
	case "answer_choice", "question", "prompt": // Если это события, требующие ответа.
		return true // Требует ответа.
	default: // Для других типов событий.
		return false // Не требует ответа.
	} // Конец switch.
} // Конец shouldRequireAnswer.

// loadAnswerDatabase загружает базу правильных ответов из онлайн-источника.
// Принимает URL для загрузки базы ответов.
// Возвращает базу ответов или ошибку.
func loadAnswerDatabase(url string) (*AnswerDatabase, error) { // Функция загрузки базы ответов.
	if url == "" { // Если URL пустой.
		return &AnswerDatabase{Games: make(map[string]GameAnswers)}, nil // Возвращаем пустую базу ответов.
	} // Конец проверки URL.

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Создаём контекст с таймаутом 10 секунд.
	defer cancel()                                                           // Отменяем контекст при выходе из функции.

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil) // Создаём HTTP GET запрос с контекстом.
	if err != nil {                                              // Если не удалось создать запрос.
		return nil, fmt.Errorf("failed to create request: %w", err) // Возвращаем ошибку.
	} // Конец проверки создания запроса.

	client := &http.Client{}    // Создаём HTTP клиент.
	resp, err := client.Do(req) // Выполняем запрос.
	if err != nil {             // Если запрос не удался.
		return nil, fmt.Errorf("failed to fetch answer database: %w", err) // Возвращаем ошибку.
	} // Конец проверки выполнения запроса.
	defer resp.Body.Close() // Закрываем тело ответа при выходе из функции.

	if resp.StatusCode != http.StatusOK { // Если статус ответа не 200.
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode) // Возвращаем ошибку.
	} // Конец проверки статуса.

	body, err := io.ReadAll(resp.Body) // Читаем всё тело ответа.
	if err != nil {                    // Если не удалось прочитать тело.
		return nil, fmt.Errorf("failed to read response body: %w", err) // Возвращаем ошибку.
	} // Конец проверки чтения тела.

	// Сначала проверяем, есть ли поле "content" (формат с массивом вопросов).
	var contentFormat TriviaDeath2ContentFormat                                                    // Переменная для формата с content.
	if err := json.Unmarshal(body, &contentFormat); err == nil && len(contentFormat.Content) > 0 { // Если удалось распарсить и есть content.
		// Это формат с content массивом - преобразуем в прямой формат вопрос->индекс.
		db := &AnswerDatabase{ // Создаём базу ответов.
			Games:     make(map[string]GameAnswers), // Инициализируем карту игр.
			Questions: make(map[string]interface{}), // Инициализируем карту вопросов.
		} // Конец создания базы.

		// Проходим по всем вопросам и находим правильный ответ.
		for _, question := range contentFormat.Content { // Проходим по каждому вопросу.
			// Нормализуем текст вопроса (убираем теги форматирования).
			normalizedText := normalizeQuestionText(question.Text) // Нормализуем текст вопроса.

			// Ищем правильный ответ в choices.
			for idx, choice := range question.Choices { // Проходим по каждому варианту ответа.
				if choice.Correct { // Если вариант правильный.
					// Сохраняем индекс правильного ответа (как строку для совместимости).
					// Используем нормализованный текст как ключ.
					db.Questions[normalizedText] = fmt.Sprintf("%d", idx) // Сохраняем индекс правильного ответа.
					break                                                 // Прерываем цикл, так как нашли правильный ответ (для обычных вопросов только один правильный).
				} // Конец проверки правильности ответа.
			} // Конец цикла по вариантам ответов.
		} // Конец цикла по вопросам.

		log.Printf("loaded answer database in content format with %d questions", len(db.Questions)) // Логируем количество вопросов.
		return db, nil                                                                              // Возвращаем базу ответов.
	} // Конец проверки формата с content.

	// Пытаемся распарсить как стандартный формат.
	var db AnswerDatabase                             // Создаём переменную для базы ответов.
	if err := json.Unmarshal(body, &db); err != nil { // Пытаемся распарсить JSON.
		return nil, fmt.Errorf("failed to parse answer database JSON: %w", err) // Возвращаем ошибку.
	} // Конец проверки парсинга.

	// Инициализируем карты, если они nil (на случай, если JSON не содержит всех полей).
	if db.Games == nil { // Если карта игр nil.
		db.Games = make(map[string]GameAnswers) // Инициализируем карту игр.
	} // Конец проверки карты игр.

	// Если это прямой формат (вопрос->ответ), парсим его отдельно.
	var directFormat map[string]interface{}                     // Переменная для прямого формата.
	if err := json.Unmarshal(body, &directFormat); err == nil { // Если удалось распарсить.
		// Проверяем, есть ли поле "games" (стандартный формат).
		if _, hasGames := directFormat["games"]; !hasGames { // Если нет поля "games", это прямой формат.
			// Сохраняем все поля как вопросы для triviadeath2.
			if db.Questions == nil { // Если карта вопросов nil.
				db.Questions = make(map[string]interface{}) // Инициализируем карту вопросов.
			} // Конец проверки карты вопросов.
			db.Questions = directFormat                                                                // Сохраняем прямые вопросы.
			log.Printf("loaded answer database in direct format with %d questions", len(db.Questions)) // Логируем количество вопросов.
		} else { // Если есть поле "games", это стандартный формат.
			log.Printf("loaded answer database with %d games", len(db.Games)) // Логируем количество загруженных игр.
		} // Конец проверки формата.
	} // Конец проверки прямого формата.

	return &db, nil // Возвращаем базу ответов.
} // Конец loadAnswerDatabase.

// loadFinalRoundDatabase загружает базу ответов для финального раунда Trivia Death 2.
// В финальном раунде может быть несколько правильных ответов для одного вопроса.
// Принимает URL для загрузки базы ответов.
// Возвращает базу ответов или ошибку.
func loadFinalRoundDatabase(url string) (*AnswerDatabase, error) { // Функция загрузки базы финального раунда.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Создаём контекст с таймаутом 30 секунд.
	defer cancel()                                                           // Отменяем контекст при выходе из функции.

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil) // Создаём HTTP GET запрос с контекстом.
	if err != nil {                                              // Если не удалось создать запрос.
		return nil, fmt.Errorf("failed to create request: %w", err) // Возвращаем ошибку.
	} // Конец проверки создания запроса.

	client := &http.Client{}    // Создаём HTTP клиент.
	resp, err := client.Do(req) // Выполняем запрос.
	if err != nil {             // Если запрос не удался.
		return nil, fmt.Errorf("failed to fetch final round database: %w", err) // Возвращаем ошибку.
	} // Конец проверки выполнения запроса.
	defer resp.Body.Close() // Закрываем тело ответа при выходе из функции.

	if resp.StatusCode != http.StatusOK { // Если статус ответа не 200.
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode) // Возвращаем ошибку.
	} // Конец проверки статуса.

	body, err := io.ReadAll(resp.Body) // Читаем всё тело ответа.
	if err != nil {                    // Если не удалось прочитать тело.
		return nil, fmt.Errorf("failed to read response body: %w", err) // Возвращаем ошибку.
	} // Конец проверки чтения тела.

	// Парсим формат с content массивом для финального раунда.
	var contentFormat TriviaDeath2ContentFormat                                                     // Переменная для формата с content.
	if err := json.Unmarshal(body, &contentFormat); err != nil || len(contentFormat.Content) == 0 { // Если не удалось распарсить или нет content.
		return nil, fmt.Errorf("invalid final round database format: %w", err) // Возвращаем ошибку.
	} // Конец проверки формата.

	// Создаём базу ответов для финального раунда.
	db := &AnswerDatabase{ // Создаём базу ответов.
		Games:               make(map[string]GameAnswers), // Инициализируем карту игр.
		Questions:           make(map[string]interface{}), // Инициализируем карту вопросов.
		FinalRoundQuestions: make(map[string][]string),    // Инициализируем карту вопросов финального раунда.
	} // Конец создания базы.

	// Проходим по всем вопросам и находим все правильные ответы.
	for _, question := range contentFormat.Content { // Проходим по каждому вопросу.
		// Нормализуем текст вопроса (убираем теги форматирования).
		normalizedText := normalizeQuestionText(question.Text) // Нормализуем текст вопроса.

		// Собираем все тексты правильных ответов.
		correctTexts := []string{}                // Слайс для текстов правильных ответов.
		for _, choice := range question.Choices { // Проходим по каждому варианту ответа.
			if choice.Correct { // Если вариант правильный.
				// Нормализуем текст ответа для сопоставления (убираем все небуквенные символы, приводим к нижнему регистру).
				normalizedText := normalizeAnswerText(choice.Text)  // Нормализуем текст ответа.
				correctTexts = append(correctTexts, normalizedText) // Добавляем нормализованный текст правильного ответа.
			} // Конец проверки правильности ответа.
		} // Конец цикла по вариантам ответов.

		// Сохраняем все тексты правильных ответов для этого вопроса.
		if len(correctTexts) > 0 { // Если есть правильные ответы.
			db.FinalRoundQuestions[normalizedText] = correctTexts // Сохраняем тексты правильных ответов.
		} // Конец проверки наличия правильных ответов.
	} // Конец цикла по вопросам.

	log.Printf("loaded final round database with %d questions", len(db.FinalRoundQuestions)) // Логируем количество вопросов финального раунда.
	return db, nil                                                                           // Возвращаем базу ответов.
} // Конец loadFinalRoundDatabase.

// getAutoAnswer получает автоматический ответ из базы данных для события.
// Принимает событие игры и базу ответов.
// Возвращает ответ и true, если ответ найден, иначе пустую строку и false.
func getAutoAnswer(event *GameEvent, db *AnswerDatabase) (string, bool) { // Функция получения автоматического ответа.
	if event == nil || db == nil { // Если событие или база ответов nil.
		return "", false // Возвращаем пустой ответ и false.
	} // Конец проверки параметров.

	// Для triviadeath2 сначала проверяем прямой формат (вопрос->ответ).
	if (event.GameTag == "triviadeath2-tjsp" || strings.Contains(event.GameTag, "triviadeath2")) && db.Questions != nil { // Если это Trivia Death 2 и есть прямые вопросы.
		// Нормализуем текст вопроса перед поиском (убираем теги форматирования).
		normalizedQuestion := normalizeQuestionText(event.EventID) // Нормализуем текст вопроса.

		// Ищем ответ по нормализованному тексту вопроса (EventID содержит текст вопроса).
		if answer, ok := db.Questions[normalizedQuestion]; ok { // Если ответ найден.
			// Преобразуем ответ в строку.
			switch v := answer.(type) { // Проверяем тип ответа.
			case string: // Если ответ - строка.
				return v, true // Возвращаем строку и true.
			case float64: // Если ответ - число (JSON числа парсятся как float64).
				return fmt.Sprintf("%.0f", v), true // Преобразуем в строку без дробной части и возвращаем true.
			case int: // Если ответ - целое число.
				return fmt.Sprintf("%d", v), true // Преобразуем в строку и возвращаем true.
			default: // Если тип ответа неизвестен.
				return fmt.Sprintf("%v", v), true // Преобразуем в строку и возвращаем true.
			} // Конец switch.
		} // Конец проверки наличия ответа.
	} // Конец проверки прямого формата.

	// Пытаемся найти ответ в стандартном формате.
	// Получаем ответы для игры.
	gameAnswers, ok := db.Games[event.GameTag] // Получаем ответы для игры по тегу.
	if !ok {                                   // Если ответы для игры не найдены.
		return "", false // Возвращаем пустой ответ и false.
	} // Конец проверки наличия ответов для игры.

	// Получаем ответы для типа события.
	eventAnswers, ok := gameAnswers.EventTypes[event.Type] // Получаем ответы для типа события.
	if !ok {                                               // Если ответы для типа события не найдены.
		return "", false // Возвращаем пустой ответ и false.
	} // Конец проверки наличия ответов для типа события.

	// Получаем ответ для конкретного вопроса по ID события.
	answer, ok := eventAnswers.Answers[event.EventID] // Получаем ответ по ID события.
	if !ok {                                          // Если ответ для вопроса не найден.
		return "", false // Возвращаем пустой ответ и false.
	} // Конец проверки наличия ответа.

	return answer, true // Возвращаем найденный ответ и true.
} // Конец getAutoAnswer.

// normalizeQuestionText нормализует текст вопроса, убирая теги форматирования.
// Удаляет теги типа [i]...[/i], [b]...[/b] и т.д., оставляя только текст внутри.
// Принимает текст вопроса.
// Возвращает нормализованный текст.
func normalizeQuestionText(text string) string { // Функция нормализации текста вопроса.
	if text == "" { // Если текст пустой.
		return text // Возвращаем пустой текст.
	} // Конец проверки пустого текста.

	normalized := text // Начинаем с исходного текста.

	// Удаляем теги форматирования типа [i]...[/i], [b]...[/b], [u]...[/u] и т.д.
	// Регулярное выражение ищет [любые_буквы]...[/любые_буквы] и заменяет на содержимое.
	tagPattern := regexp.MustCompile(`\[/?[a-zA-Z]+\]`)      // Создаём регулярное выражение для поиска тегов.
	normalized = tagPattern.ReplaceAllString(normalized, "") // Удаляем все теги.

	// Убираем лишние пробелы (двойные, тройные и т.д.).
	normalized = strings.Join(strings.Fields(normalized), " ") // Нормализуем пробелы.

	// Убираем пробелы в начале и конце.
	normalized = strings.TrimSpace(normalized) // Убираем пробелы по краям.

	return normalized // Возвращаем нормализованный текст.
} // Конец normalizeQuestionText.

// normalizeAnswerText нормализует текст ответа для сопоставления.
// Убирает все пробелы, дефисы и другие небуквенные символы, приводит к нижнему регистру.
// Принимает текст ответа.
// Возвращает нормализованный текст (только буквы в нижнем регистре).
func normalizeAnswerText(text string) string { // Функция нормализации текста ответа.
	if text == "" { // Если текст пустой.
		return text // Возвращаем пустой текст.
	} // Конец проверки пустого текста.

	normalized := strings.ToLower(text) // Приводим к нижнему регистру.

	// Убираем все небуквенные символы (пробелы, дефисы, точки и т.д.), оставляя только буквы.
	// Используем регулярное выражение для удаления всех символов, кроме букв.
	nonLetterPattern := regexp.MustCompile(`[^а-яёa-z]`)           // Создаём регулярное выражение для поиска небуквенных символов (включая русские буквы).
	normalized = nonLetterPattern.ReplaceAllString(normalized, "") // Удаляем все небуквенные символы.

	return normalized // Возвращаем нормализованный текст.
} // Конец normalizeAnswerText.

// getFinalRoundAnswers получает все тексты правильных ответов для финального раунда.
// В финальном раунде может быть несколько правильных ответов.
// Принимает событие игры и базу ответов финального раунда.
// Возвращает слайс текстов правильных ответов и флаг успеха.
func getFinalRoundAnswers(event *GameEvent, db *AnswerDatabase) ([]string, bool) { // Функция получения ответов финального раунда.
	if event == nil || db == nil { // Если событие или база nil.
		return nil, false // Возвращаем nil и false.
	} // Конец проверки параметров.

	if db.FinalRoundQuestions == nil { // Если база вопросов финального раунда не инициализирована.
		return nil, false // Возвращаем nil и false.
	} // Конец проверки базы.

	// Нормализуем текст вопроса для поиска в базе.
	normalizedQuestion := normalizeQuestionText(event.EventID) // Нормализуем текст вопроса.

	// Ищем вопрос в базе финального раунда.
	if correctTexts, ok := db.FinalRoundQuestions[normalizedQuestion]; ok { // Если вопрос найден.
		return correctTexts, true // Возвращаем тексты правильных ответов и true.
	} // Конец проверки наличия вопроса.

	return nil, false // Возвращаем nil и false, если вопрос не найден.
} // Конец getFinalRoundAnswers.
