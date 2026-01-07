package commands // Пакет commands содержит реализацию CLI команд.

import ( // Начинаем блок импортов.
	"bufio"   // Читаем ввод пользователя из stdin.
	"fmt"     // Форматируем сообщения и ошибки.
	"log"     // Логируем события.
	"os"      // Работаем с stdin для ввода пользователя.
	"strconv" // Преобразуем строки в числа для индексов ответов.
	"strings" // Работаем со строками для обработки ввода.
) // Закрываем блок импортов.

// handleEvent обрабатывает событие игры.
// Определяет, нужен ли ответ, и отправляет команды клиентам.
// Принимает событие игры и менеджер ботнета.
// Возвращает ошибку, если обработка не удалась.
func handleEvent(event *GameEvent, manager *BotnetManager) error { // Функция обработки события.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	log.Printf("coordinator: received event type=%s, gameTag=%s, eventID=%s, requiresAnswer=%v", event.Type, event.GameTag, event.EventID, event.RequiresAnswer) // Логируем получение события.

	// Если событие не требует ответа, просто логируем его.
	if !event.RequiresAnswer { // Если ответ не требуется.
		return nil // Возвращаем nil (ошибки нет).
	} // Конец проверки необходимости ответа.

	// Определяем, какая игра требует обработки.
	switch { // Проверяем тег игры.
	case event.GameTag == "triviadeath2-tjsp" || strings.Contains(event.GameTag, "triviadeath2"): // Если это Trivia Death 2.
		return handleTriviaDeath2Event(event, manager) // Обрабатываем событие Trivia Death 2.
	case event.GameTag == "quiplash2" || event.GameTag == "quiplash": // Если это Quiplash 2 или Quiplash.
		return handleQuiplash2Event(event, manager) // Обрабатываем событие Quiplash 2.
	case event.GameTag == "everyday": // Если это игра "everyday".
		return handleEverydayEvent(event, manager) // Обрабатываем событие Everyday.
	default: // Для других игр.
		log.Printf("coordinator: unknown game tag %s, using generic handler", event.GameTag) // Логируем неизвестный тег игры.
		return handleGenericEvent(event, manager)                                            // Используем общий обработчик.
	} // Конец switch.
} // Конец handleEvent.

// handleTriviaDeath2Event обрабатывает события игры Trivia Death 2.
// Принимает событие игры и менеджер ботнета.
// Возвращает ошибку, если обработка не удалась.
func handleTriviaDeath2Event(event *GameEvent, manager *BotnetManager) error { // Функция обработки событий Trivia Death 2.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	log.Printf("coordinator: handling Trivia Death 2 event type=%s, eventID=%s", event.Type, event.EventID) // Логируем обработку события Trivia Death 2.

	// Извлекаем информацию о вопросе и вариантах ответов из entities.audiencePlayer.
	questionInfo := extractTriviaDeath2Question(event) // Извлекаем информацию о вопросе.
	if questionInfo == nil {                           // Если информация о вопросе не найдена.
		// Добавляем отладочную информацию для понимания, почему вопрос не найден.
		if event.Payload != nil { // Если payload не nil.
			if key, ok := event.Payload["key"].(string); ok { // Если есть key.
				log.Printf("coordinator: no question found, payload key=%s", key) // Логируем key.
				if val, ok := event.Payload["val"].(map[string]interface{}); ok { // Если есть val.
					if hasSubmit, ok := val["hasSubmit"].(bool); ok { // Если есть hasSubmit.
						log.Printf("coordinator: hasSubmit=%v", hasSubmit) // Логируем hasSubmit.
					} // Конец проверки hasSubmit.
					if roundType, ok := val["roundType"].(string); ok { // Если есть roundType.
						log.Printf("coordinator: roundType=%s", roundType) // Логируем roundType.
					} // Конец проверки roundType.
					if prompt, ok := val["prompt"].(string); ok { // Если есть prompt.
						log.Printf("coordinator: prompt=%s", prompt) // Логируем prompt.
					} // Конец проверки prompt.
					if choices, ok := val["choices"].([]interface{}); ok { // Если есть choices.
						log.Printf("coordinator: choices count=%d", len(choices)) // Логируем количество choices.
					} // Конец проверки choices.
				} // Конец проверки val.
			} // Конец проверки key.
		} // Конец проверки payload.
		log.Printf("coordinator: no question found in Trivia Death 2 event") // Логируем отсутствие вопроса.
		return nil                                                           // Возвращаем nil (ошибки нет, просто нет вопроса для ответа).
	} // Конец проверки информации о вопросе.

	log.Printf("coordinator: Trivia Death 2 question: %s, choices: %d, roundType: %s", questionInfo.Prompt, len(questionInfo.Choices), questionInfo.RoundType) // Логируем информацию о вопросе.

	// Проверяем, является ли это финальным раундом.
	if questionInfo.RoundType == "FinalRound" { // Если это финальный раунд.
		log.Printf("coordinator: detected Final Round question") // Логируем обнаружение финального раунда.
		// Используем базу финального раунда для получения всех правильных ответов.
		event.EventID = questionInfo.Prompt                                      // Устанавливаем EventID как текст вопроса для поиска в базе.
		correctTexts, found := getFinalRoundAnswers(event, manager.finalRoundDB) // Получаем все тексты правильных ответов.
		if found {                                                               // Если ответы найдены.
			// Сопоставляем тексты вариантов ответов из вопроса с текстами правильных ответов из базы данных.
			correctIndices := []int{}                           // Слайс для индексов правильных ответов в вопросе от сервера.
			questionChoicesNormalized := []string{}             // Слайс для нормализованных текстов вариантов ответов из вопроса (для логирования).
			for idx, choiceText := range questionInfo.Choices { // Проходим по каждому варианту ответа из вопроса.
				normalizedChoiceText := normalizeAnswerText(choiceText)                             // Нормализуем текст варианта ответа из вопроса.
				questionChoicesNormalized = append(questionChoicesNormalized, normalizedChoiceText) // Сохраняем нормализованный текст для логирования.
				for _, correctText := range correctTexts {                                          // Проходим по каждому правильному тексту из базы данных.
					if normalizedChoiceText == correctText { // Если тексты совпадают (уже нормализованы).
						correctIndices = append(correctIndices, idx) // Добавляем индекс правильного ответа.
						break                                        // Прерываем внутренний цикл, так как нашли совпадение.
					} // Конец проверки совпадения текстов.
				} // Конец цикла по правильным текстам из базы данных.
			} // Конец цикла по вариантам ответов из вопроса.
			if len(correctIndices) == 0 { // Если после сопоставления не осталось индексов.
				// Вопрос найден в базе данных, но правильные ответы из базы не совпадают с вариантами в вопросе.
				// В этом случае отправляем пустой массив индексов [] клиентам.
				log.Printf("coordinator: no matching answers found for Final Round (question found in DB, but correct answers don't match question choices)") // Логируем отсутствие совпадений.
				log.Printf("coordinator: question choices (normalized): %v", questionChoicesNormalized)                                                       // Логируем нормализованные варианты ответов из вопроса.
				log.Printf("coordinator: correct texts from DB (normalized): %v", correctTexts)                                                               // Логируем нормализованные правильные тексты из базы данных.
				log.Printf("coordinator: original question choices: %v", questionInfo.Choices)                                                                // Логируем оригинальные варианты ответов из вопроса.
				log.Printf("coordinator: sending empty answer [] to clients (no matches found)")                                                              // Логируем отправку пустого ответа.
				// Отправляем пустой массив индексов клиентам.
				return sendTriviaDeath2FinalRoundAnswerToAllClients(event, []int{}, manager) // Отправляем пустой массив индексов всем клиентам.
			} // Конец проверки наличия совпадений.
			log.Printf("coordinator: found auto-answers for Final Round: %v (matched from DB texts: %v)", correctIndices, correctTexts) // Логируем найденные индексы после сопоставления.
			// Отправляем команду всем клиентам с множественными индексами ответов.
			return sendTriviaDeath2FinalRoundAnswerToAllClients(event, correctIndices, manager) // Отправляем ответы всем клиентам.
		} else { // Если ответы не найдены.
			log.Printf("coordinator: no auto-answers found for Final Round, prompting user") // Логируем отсутствие автоматических ответов.
			// Запрашиваем ответ у пользователя для финального раунда.
			userAnswers, err := promptUserForTriviaDeath2FinalRoundAnswer(event, questionInfo) // Запрашиваем ответы у пользователя.
			if err != nil {                                                                    // Если запрос ответов не удался.
				return fmt.Errorf("failed to get user answers for final round: %w", err) // Возвращаем ошибку.
			} // Конец проверки запроса ответов.

			if len(userAnswers) == 0 { // Если пользователь не выбрал ответы.
				log.Printf("coordinator: user did not provide answers for Final Round, skipping") // Логируем пропуск ответов.
				return nil                                                                        // Возвращаем nil (ошибки нет).
			} // Конец проверки ответов пользователя.

			log.Printf("coordinator: user provided answers for Final Round: %v", userAnswers) // Логируем ответы пользователя.
			// Отправляем ответы всем клиентам.
			return sendTriviaDeath2FinalRoundAnswerToAllClients(event, userAnswers, manager) // Отправляем ответы всем клиентам.
		} // Конец проверки наличия автоматических ответов.
	} // Конец проверки финального раунда.

	// Обычный раунд - используем стандартную логику с одним правильным ответом.
	// Пытаемся получить автоматический ответ из базы данных.
	// Используем prompt как ключ для поиска ответа.
	event.EventID = questionInfo.Prompt                     // Устанавливаем EventID как текст вопроса для поиска в базе.
	answer, found := getAutoAnswer(event, manager.answerDB) // Получаем автоматический ответ.
	if found {                                              // Если ответ найден.
		log.Printf("coordinator: found auto-answer for Trivia Death 2: %s", answer) // Логируем найденный автоматический ответ.
		// Пытаемся преобразовать ответ в индекс (если это число).
		var answerIndex int                               // Переменная для индекса ответа.
		if idx, err := strconv.Atoi(answer); err == nil { // Если ответ - это число.
			answerIndex = idx // Используем число как индекс.
		} else { // Если ответ не число, ищем текст в choices.
			answerIndex = findAnswerIndex(answer, questionInfo.Choices) // Находим индекс ответа по тексту.
		} // Конец проверки формата ответа.

		if answerIndex >= 0 && answerIndex < len(questionInfo.Choices) { // Если индекс найден и в допустимом диапазоне.
			// Отправляем команду всем клиентам с индексом ответа.
			return sendTriviaDeath2AnswerToAllClients(event, answerIndex, manager) // Отправляем ответ всем клиентам.
		} else { // Если индекс не найден или вне диапазона.
			log.Printf("coordinator: answer '%s' (index %d) not found in choices or out of range, prompting user", answer, answerIndex) // Логируем отсутствие ответа в вариантах.
		} // Конец проверки индекса.
	} // Конец проверки наличия автоматического ответа.

	// Если автоматический ответ не найден, запрашиваем у пользователя.
	log.Printf("coordinator: no auto-answer found for Trivia Death 2, prompting user") // Логируем отсутствие автоматического ответа.

	// Запрашиваем ответ у пользователя.
	userAnswer, err := promptUserForTriviaDeath2Answer(event, questionInfo) // Запрашиваем ответ у пользователя.
	if err != nil {                                                         // Если запрос ответа не удался.
		return fmt.Errorf("failed to get user answer: %w", err) // Возвращаем ошибку.
	} // Конец проверки запроса ответа.

	if userAnswer < 0 { // Если пользователь не выбрал ответ.
		log.Printf("coordinator: user did not provide answer, skipping") // Логируем пропуск ответа.
		return nil                                                       // Возвращаем nil (ошибки нет).
	} // Конец проверки ответа пользователя.

	log.Printf("coordinator: user provided answer index: %d", userAnswer) // Логируем ответ пользователя.

	// Отправляем ответ всем клиентам.
	return sendTriviaDeath2AnswerToAllClients(event, userAnswer, manager) // Отправляем ответ всем клиентам.
} // Конец handleTriviaDeath2Event.

// TriviaDeath2QuestionInfo содержит информацию о вопросе Trivia Death 2.
type TriviaDeath2QuestionInfo struct { // Структура информации о вопросе.
	Prompt    string   // Текст вопроса.
	Choices   []string // Варианты ответов.
	RoundType string   // Тип раунда (например, "FinalRound" для финального раунда).
} // Конец TriviaDeath2QuestionInfo.

// extractTriviaDeath2Question извлекает информацию о вопросе из события Trivia Death 2.
// Принимает событие игры.
// Возвращает информацию о вопросе или nil.
func extractTriviaDeath2Question(event *GameEvent) *TriviaDeath2QuestionInfo { // Функция извлечения информации о вопросе.
	if event == nil || event.Payload == nil { // Если событие или payload nil.
		return nil // Возвращаем nil.
	} // Конец проверки параметров.

	// Формат 1: opcode "object" с result.key == "audiencePlayer" и result.val.
	if event.Type == "object" { // Если opcode = "object".
		if key, ok := event.Payload["key"].(string); ok && key == "audiencePlayer" { // Если key = "audiencePlayer".
			if val, ok := event.Payload["val"].(map[string]interface{}); ok { // Если есть val.
				// Проверяем roundType перед проверкой hasSubmit.
				roundType := ""                              // Переменная для типа раунда.
				if rt, ok := val["roundType"].(string); ok { // Если есть roundType.
					roundType = rt // Устанавливаем тип раунда.
				} // Конец проверки roundType.

				// Для финального раунда игнорируем hasSubmit, так как он может быть true даже когда вопрос активен.
				// Для обычного раунда проверяем hasSubmit только если он false (вопрос активен).
				if roundType != "FinalRound" { // Если это не финальный раунд.
					if hasSubmit, ok := val["hasSubmit"].(bool); ok && hasSubmit { // Если hasSubmit = true (вопрос уже отвечен).
						log.Printf("coordinator: extractTriviaDeath2Question: skipping question with hasSubmit=true (not FinalRound)") // Логируем пропуск вопроса.
						return nil                                                                                                     // Возвращаем nil (вопрос уже отвечен).
					} // Конец проверки hasSubmit.
				} else { // Если это финальный раунд.
					log.Printf("coordinator: extractTriviaDeath2Question: FinalRound detected, ignoring hasSubmit") // Логируем игнорирование hasSubmit для финального раунда.
				} // Конец проверки финального раунда.

				// Извлекаем prompt (текст вопроса).
				prompt := ""                             // Переменная для текста вопроса.
				if p, ok := val["prompt"].(string); ok { // Если есть prompt.
					prompt = p // Устанавливаем текст вопроса.
				} // Конец проверки prompt.

				// Извлекаем choices (варианты ответов).
				choices := []string{}                                      // Слайс для вариантов ответов.
				if choicesData, ok := val["choices"].([]interface{}); ok { // Если есть choices.
					for _, choiceItem := range choicesData { // Проходим по каждому варианту.
						if choiceMap, ok := choiceItem.(map[string]interface{}); ok { // Если вариант - это map.
							if text, ok := choiceMap["text"].(string); ok { // Если есть text.
								choices = append(choices, text) // Добавляем вариант ответа.
							} // Конец проверки text.
						} // Конец проверки choiceMap.
					} // Конец цикла по вариантам.
				} // Конец проверки choices.

				// roundType уже извлечён выше, используем его.
				if prompt != "" && len(choices) > 0 { // Если есть вопрос и варианты ответов.
					return &TriviaDeath2QuestionInfo{ // Возвращаем информацию о вопросе.
						Prompt:    prompt,    // Устанавливаем текст вопроса.
						Choices:   choices,   // Устанавливаем варианты ответов.
						RoundType: roundType, // Устанавливаем тип раунда.
					} // Конец создания информации о вопросе.
				} // Конец проверки наличия вопроса и вариантов.
			} // Конец проверки val.
		} // Конец проверки key.
	} // Конец проверки opcode "object".

	// Формат 2: entities.audiencePlayer[1].val (старый формат).
	if entities, ok := event.Payload["entities"].(map[string]interface{}); ok { // Если есть entities.
		if audiencePlayer, ok := entities["audiencePlayer"].([]interface{}); ok && len(audiencePlayer) > 1 { // Если есть audiencePlayer.
			if playerData, ok := audiencePlayer[1].(map[string]interface{}); ok { // Если playerData - это map.
				if playerVal, ok := playerData["val"].(map[string]interface{}); ok { // Если есть val.
					// Проверяем hasSubmit.
					if hasSubmit, ok := playerVal["hasSubmit"].(bool); ok && hasSubmit { // Если hasSubmit = true (вопрос уже отвечен).
						return nil // Возвращаем nil (вопрос уже отвечен).
					} // Конец проверки hasSubmit.

					// Извлекаем prompt (текст вопроса).
					prompt := ""                                   // Переменная для текста вопроса.
					if p, ok := playerVal["prompt"].(string); ok { // Если есть prompt.
						prompt = p // Устанавливаем текст вопроса.
					} // Конец проверки prompt.

					// Извлекаем choices (варианты ответов).
					choices := []string{}                                            // Слайс для вариантов ответов.
					if choicesData, ok := playerVal["choices"].([]interface{}); ok { // Если есть choices.
						for _, choiceItem := range choicesData { // Проходим по каждому варианту.
							if choiceMap, ok := choiceItem.(map[string]interface{}); ok { // Если вариант - это map.
								if text, ok := choiceMap["text"].(string); ok { // Если есть text.
									choices = append(choices, text) // Добавляем вариант ответа.
								} // Конец проверки text.
							} // Конец проверки choiceMap.
						} // Конец цикла по вариантам.
					} // Конец проверки choices.

					// Извлекаем roundType (тип раунда) для старого формата.
					roundType := ""                                    // Переменная для типа раунда.
					if rt, ok := playerVal["roundType"].(string); ok { // Если есть roundType.
						roundType = rt // Устанавливаем тип раунда.
					} // Конец проверки roundType.

					if prompt != "" && len(choices) > 0 { // Если есть вопрос и варианты ответов.
						return &TriviaDeath2QuestionInfo{ // Возвращаем информацию о вопросе.
							Prompt:    prompt,    // Устанавливаем текст вопроса.
							Choices:   choices,   // Устанавливаем варианты ответов.
							RoundType: roundType, // Устанавливаем тип раунда.
						} // Конец создания информации о вопросе.
					} // Конец проверки наличия вопроса и вариантов.
				} // Конец проверки val.
			} // Конец проверки playerData.
		} // Конец проверки audiencePlayer.
	} // Конец проверки entities.

	return nil // Возвращаем nil, если информация о вопросе не найдена.
} // Конец extractTriviaDeath2Question.

// findAnswerIndex находит индекс ответа в списке вариантов.
// Принимает текст ответа и список вариантов.
// Возвращает индекс ответа или -1, если не найден.
func findAnswerIndex(answer string, choices []string) int { // Функция поиска индекса ответа.
	answerLower := strings.ToLower(strings.TrimSpace(answer)) // Приводим ответ к нижнему регистру и убираем пробелы.

	for i, choice := range choices { // Проходим по каждому варианту.
		choiceLower := strings.ToLower(strings.TrimSpace(choice)) // Приводим вариант к нижнему регистру и убираем пробелы.
		if answerLower == choiceLower {                           // Если ответ совпадает с вариантом.
			return i // Возвращаем индекс.
		} // Конец проверки совпадения.
	} // Конец цикла.

	return -1 // Возвращаем -1, если ответ не найден.
} // Конец findAnswerIndex.

// promptUserForTriviaDeath2Answer запрашивает ответ у пользователя для Trivia Death 2.
// Принимает событие игры и информацию о вопросе.
// Возвращает индекс выбранного ответа или -1, если пользователь не выбрал.
func promptUserForTriviaDeath2Answer(event *GameEvent, questionInfo *TriviaDeath2QuestionInfo) (int, error) { // Функция запроса ответа у пользователя.
	if event == nil || questionInfo == nil { // Если событие или информация о вопросе nil.
		return -1, fmt.Errorf("event or questionInfo is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	// Выводим информацию о вопросе.
	fmt.Printf("\n=== Trivia Death 2 - Answer Required ===\n") // Выводим заголовок.
	fmt.Printf("Question: %s\n", questionInfo.Prompt)          // Выводим текст вопроса.
	fmt.Printf("Choices:\n")                                   // Выводим заголовок вариантов.

	for i, choice := range questionInfo.Choices { // Проходим по каждому варианту.
		fmt.Printf("  %d. %s\n", i, choice) // Выводим номер и текст варианта.
	} // Конец цикла.

	fmt.Printf("Enter choice number (0-%d) or press Enter to skip: ", len(questionInfo.Choices)-1) // Выводим подсказку для ввода.

	// Читаем ввод пользователя.
	scanner := bufio.NewScanner(os.Stdin) // Создаём сканер для чтения из stdin.
	if !scanner.Scan() {                  // Если чтение не удалось.
		return -1, fmt.Errorf("failed to read user input") // Возвращаем ошибку.
	} // Конец проверки чтения.

	input := strings.TrimSpace(scanner.Text()) // Получаем введённый текст и убираем пробелы.

	if err := scanner.Err(); err != nil { // Если произошла ошибка сканера.
		return -1, fmt.Errorf("scanner error: %w", err) // Возвращаем ошибку.
	} // Конец проверки ошибки сканера.

	if input == "" { // Если пользователь не ввёл ничего.
		return -1, nil // Возвращаем -1 и nil (ошибки нет).
	} // Конец проверки пустого ввода.

	// Преобразуем ввод в число.
	choiceIndex, err := strconv.Atoi(input) // Преобразуем строку в число.
	if err != nil {                         // Если преобразование не удалось.
		return -1, fmt.Errorf("invalid choice number: %s", input) // Возвращаем ошибку.
	} // Конец проверки преобразования.

	// Проверяем, что индекс в допустимом диапазоне.
	if choiceIndex < 0 || choiceIndex >= len(questionInfo.Choices) { // Если индекс вне диапазона.
		return -1, fmt.Errorf("choice number out of range: %d (must be 0-%d)", choiceIndex, len(questionInfo.Choices)-1) // Возвращаем ошибку.
	} // Конец проверки диапазона.

	return choiceIndex, nil // Возвращаем индекс выбранного ответа и nil (ошибки нет).
} // Конец promptUserForTriviaDeath2Answer.

// promptUserForTriviaDeath2FinalRoundAnswer запрашивает ответы у пользователя для финального раунда Trivia Death 2.
// В финальном раунде может быть несколько правильных ответов.
// Принимает событие игры и информацию о вопросе.
// Возвращает слайс индексов выбранных ответов или пустой слайс, если пользователь не выбрал.
func promptUserForTriviaDeath2FinalRoundAnswer(event *GameEvent, questionInfo *TriviaDeath2QuestionInfo) ([]int, error) { // Функция запроса ответов у пользователя для финального раунда.
	if event == nil || questionInfo == nil { // Если событие или информация о вопросе nil.
		return nil, fmt.Errorf("event or questionInfo is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	// Выводим информацию о вопросе.
	fmt.Printf("\n=== Trivia Death 2 - Final Round - Answer Required ===\n") // Выводим заголовок.
	fmt.Printf("Question: %s\n", questionInfo.Prompt)                        // Выводим текст вопроса.
	fmt.Printf("Choices:\n")                                                 // Выводим заголовок вариантов.

	for i, choice := range questionInfo.Choices { // Проходим по каждому варианту.
		fmt.Printf("  %d. %s\n", i, choice) // Выводим номер и текст варианта.
	} // Конец цикла.

	fmt.Printf("Enter choice numbers separated by commas (e.g., 0,1,2) or press Enter to skip: ") // Выводим подсказку для ввода.

	// Читаем ввод пользователя.
	scanner := bufio.NewScanner(os.Stdin) // Создаём сканер для чтения из stdin.
	if !scanner.Scan() {                  // Если чтение не удалось.
		return nil, fmt.Errorf("failed to read user input") // Возвращаем ошибку.
	} // Конец проверки чтения.

	input := strings.TrimSpace(scanner.Text()) // Получаем введённый текст и убираем пробелы.

	if err := scanner.Err(); err != nil { // Если произошла ошибка сканера.
		return nil, fmt.Errorf("scanner error: %w", err) // Возвращаем ошибку.
	} // Конец проверки ошибки сканера.

	if input == "" { // Если пользователь не ввёл ничего.
		return []int{}, nil // Возвращаем пустой слайс и nil (ошибки нет).
	} // Конец проверки пустого ввода.

	// Разбиваем ввод по запятым и преобразуем в числа.
	parts := strings.Split(input, ",") // Разбиваем строку по запятым.
	indices := []int{}                 // Слайс для индексов.

	for _, part := range parts { // Проходим по каждой части.
		part = strings.TrimSpace(part) // Убираем пробелы.
		if part == "" {                // Если часть пустая.
			continue // Пропускаем.
		} // Конец проверки пустой части.

		// Преобразуем в число.
		choiceIndex, err := strconv.Atoi(part) // Преобразуем строку в число.
		if err != nil {                        // Если преобразование не удалось.
			return nil, fmt.Errorf("invalid choice number: %s", part) // Возвращаем ошибку.
		} // Конец проверки преобразования.

		// Проверяем, что индекс в допустимом диапазоне.
		if choiceIndex < 0 || choiceIndex >= len(questionInfo.Choices) { // Если индекс вне диапазона.
			return nil, fmt.Errorf("choice number out of range: %d (must be 0-%d)", choiceIndex, len(questionInfo.Choices)-1) // Возвращаем ошибку.
		} // Конец проверки диапазона.

		// Проверяем, что индекс ещё не добавлен (избегаем дубликатов).
		alreadyAdded := false         // Флаг наличия индекса.
		for _, idx := range indices { // Проходим по уже добавленным индексам.
			if idx == choiceIndex { // Если индекс уже есть.
				alreadyAdded = true // Устанавливаем флаг.
				break               // Прерываем цикл.
			} // Конец проверки совпадения.
		} // Конец цикла проверки дубликатов.

		if !alreadyAdded { // Если индекс ещё не добавлен.
			indices = append(indices, choiceIndex) // Добавляем индекс.
		} // Конец проверки дубликата.
	} // Конец цикла по частям.

	return indices, nil // Возвращаем слайс индексов и nil (ошибки нет).
} // Конец promptUserForTriviaDeath2FinalRoundAnswer.

// sendTriviaDeath2AnswerToAllClients отправляет ответ Trivia Death 2 всем клиентам.
// Принимает событие игры, индекс ответа и менеджер ботнета.
// Возвращает ошибку, если отправка не удалась.
func sendTriviaDeath2AnswerToAllClients(event *GameEvent, answerIndex int, manager *BotnetManager) error { // Функция отправки ответа всем клиентам.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	if answerIndex < 0 { // Если индекс ответа отрицательный.
		return fmt.Errorf("answer index is negative") // Возвращаем ошибку.
	} // Конец проверки индекса.

	// Создаём команду для клиентов.
	// Для Trivia Death 2 нужно отправить индекс выбранного ответа.
	cmd := ClientCommand{ // Создаём команду.
		Type:    "answer",                       // Устанавливаем тип команды.
		EventID: event.EventID,                  // Устанавливаем ID события.
		Answer:  fmt.Sprintf("%d", answerIndex), // Устанавливаем ответ как строковое представление индекса.
		Payload: make(map[string]interface{}),   // Инициализируем payload.
	} // Конец создания команды.

	// Добавляем дополнительную информацию в payload.
	cmd.Payload["gameTag"] = event.GameTag   // Добавляем тег игры.
	cmd.Payload["eventType"] = event.Type    // Добавляем тип события.
	cmd.Payload["answerIndex"] = answerIndex // Добавляем индекс ответа.

	// Отправляем команду всем подключенным клиентам через канал.
	manager.mu.RLock()                  // Блокируем мьютекс для чтения.
	clientCount := len(manager.clients) // Получаем количество подключенных клиентов.
	manager.mu.RUnlock()                // Разблокируем мьютекс.

	if clientCount == 0 { // Если клиентов ещё нет.
		log.Printf("coordinator: no clients connected yet, skipping answer") // Логируем пропуск ответа.
		return nil                                                           // Возвращаем nil (ошибки нет, просто клиентов нет).
	} // Конец проверки количества клиентов.

	log.Printf("coordinator: sending Trivia Death 2 answer (index %d) to %d clients", answerIndex, clientCount) // Логируем отправку ответа.

	// Отправляем команду в канал для каждого подключенного клиента.
	// Каждый клиент слушает канал и получит команду.
	for i := 0; i < clientCount; i++ { // Проходим по количеству клиентов.
		select { // Выбираем между контекстом и отправкой команды.
		case <-manager.ctx.Done(): // Если контекст отменён.
			return fmt.Errorf("context canceled") // Возвращаем ошибку.
		case manager.commandChan <- cmd: // Если команда отправлена в канал.
			// Команда успешно отправлена.
		} // Конец select.
	} // Конец цикла отправки.

	log.Printf("coordinator: Trivia Death 2 answer sent to all clients") // Логируем успешную отправку.

	return nil // Возвращаем nil (ошибки нет).
} // Конец sendTriviaDeath2AnswerToAllClients.

// sendTriviaDeath2FinalRoundAnswerToAllClients отправляет ответы финального раунда Trivia Death 2 всем клиентам.
// В финальном раунде может быть несколько правильных ответов.
// Принимает событие игры, слайс индексов ответов и менеджер ботнета.
// Возвращает ошибку, если отправка не удалась.
func sendTriviaDeath2FinalRoundAnswerToAllClients(event *GameEvent, answerIndices []int, manager *BotnetManager) error { // Функция отправки ответов финального раунда всем клиентам.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	// Создаём строку с индексами через запятую (например, "1,2" или "" для пустого массива).
	var voteString string        // Переменная для строки с индексами.
	if len(answerIndices) == 0 { // Если индексы ответов пусты.
		voteString = "" // Устанавливаем пустую строку.
	} else { // Если есть индексы.
		indexStrings := make([]string, len(answerIndices)) // Создаём слайс строк для индексов.
		for i, idx := range answerIndices {                // Проходим по каждому индексу.
			indexStrings[i] = fmt.Sprintf("%d", idx) // Преобразуем индекс в строку.
		} // Конец цикла.
		voteString = strings.Join(indexStrings, ",") // Объединяем индексы через запятую.
	} // Конец проверки наличия индексов.

	// Создаём команду для клиентов.
	// Для финального раунда Trivia Death 2 нужно отправить строку с индексами через запятую.
	cmd := ClientCommand{ // Создаём команду.
		Type:    "answer",                     // Устанавливаем тип команды.
		EventID: event.EventID,                // Устанавливаем ID события.
		Answer:  voteString,                   // Устанавливаем ответ как строку с индексами через запятую.
		Payload: make(map[string]interface{}), // Инициализируем payload.
	} // Конец создания команды.

	// Добавляем дополнительную информацию в payload.
	cmd.Payload["gameTag"] = event.GameTag       // Добавляем тег игры.
	cmd.Payload["eventType"] = event.Type        // Добавляем тип события.
	cmd.Payload["answerIndices"] = answerIndices // Добавляем слайс индексов ответов.
	cmd.Payload["isFinalRound"] = true           // Устанавливаем флаг финального раунда.

	// Отправляем команду всем подключенным клиентам через канал.
	manager.mu.RLock()                  // Блокируем мьютекс для чтения.
	clientCount := len(manager.clients) // Получаем количество подключенных клиентов.
	manager.mu.RUnlock()                // Разблокируем мьютекс.

	if clientCount == 0 { // Если клиентов ещё нет.
		log.Printf("coordinator: no clients connected yet, skipping final round answer") // Логируем пропуск ответа.
		return nil                                                                       // Возвращаем nil (ошибки нет, просто клиентов нет).
	} // Конец проверки количества клиентов.

	log.Printf("coordinator: sending Trivia Death 2 Final Round answer (indices %v) to %d clients", answerIndices, clientCount) // Логируем отправку ответов.

	// Отправляем команду в канал для каждого подключенного клиента.
	// Каждый клиент слушает канал и получит команду.
	for i := 0; i < clientCount; i++ { // Проходим по количеству клиентов.
		select { // Выбираем между контекстом и отправкой команды.
		case <-manager.ctx.Done(): // Если контекст отменён.
			return fmt.Errorf("context canceled") // Возвращаем ошибку.
		case manager.commandChan <- cmd: // Если команда отправлена в канал.
			// Команда успешно отправлена.
		} // Конец select.
	} // Конец цикла отправки.

	log.Printf("coordinator: Trivia Death 2 Final Round answer sent to all clients") // Логируем успешную отправку.

	return nil // Возвращаем nil (ошибки нет).
} // Конец sendTriviaDeath2FinalRoundAnswerToAllClients.

// sendEverydayAnswerToAllClients отправляет ответ Everyday всем клиентам.
// Принимает событие игры, key и times, и менеджер ботнета.
// Возвращает ошибку, если отправка не удалась.
func sendEverydayAnswerToAllClients(event *GameEvent, key string, times int, manager *BotnetManager) error { // Функция отправки ответа Everyday всем клиентам.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	if key == "" { // Если key пустой.
		return fmt.Errorf("key is empty") // Возвращаем ошибку.
	} // Конец проверки key.

	// Создаём команду для клиентов.
	// Для Everyday нужно отправить key и times.
	cmd := ClientCommand{ // Создаём команду.
		Type:    "answer",                     // Устанавливаем тип команды.
		EventID: event.EventID,                // Устанавливаем ID события.
		Answer:  key,                          // Устанавливаем ответ как key.
		Payload: make(map[string]interface{}), // Инициализируем payload.
	} // Конец создания команды.

	// Добавляем дополнительную информацию в payload.
	cmd.Payload["gameTag"] = event.GameTag                 // Добавляем тег игры.
	cmd.Payload["eventType"] = event.Type                  // Добавляем тип события.
	cmd.Payload["key"] = key                               // Добавляем key.
	cmd.Payload["times"] = times                           // Добавляем times.
	cmd.Payload["opcode"] = "audience/g-counter/increment" // Добавляем opcode для Everyday.

	// Отправляем команду всем подключенным клиентам через канал.
	manager.mu.RLock()                  // Блокируем мьютекс для чтения.
	clientCount := len(manager.clients) // Получаем количество подключенных клиентов.
	manager.mu.RUnlock()                // Разблокируем мьютекс.

	if clientCount == 0 { // Если клиентов ещё нет.
		log.Printf("coordinator: no clients connected yet, skipping Everyday answer") // Логируем пропуск ответа.
		return nil                                                                    // Возвращаем nil (ошибки нет, просто клиентов нет).
	} // Конец проверки количества клиентов.

	log.Printf("coordinator: sending Everyday answer (key=%s, times=%d) to %d clients", key, times, clientCount) // Логируем отправку ответа.

	// Отправляем команду в канал для каждого подключенного клиента.
	// Каждый клиент слушает канал и получит команду.
	for i := 0; i < clientCount; i++ { // Проходим по количеству клиентов.
		select { // Выбираем между контекстом и отправкой команды.
		case <-manager.ctx.Done(): // Если контекст отменён.
			return fmt.Errorf("context canceled") // Возвращаем ошибку.
		case manager.commandChan <- cmd: // Если команда отправлена в канал.
			// Команда успешно отправлена.
		} // Конец select.
	} // Конец цикла отправки.

	log.Printf("coordinator: Everyday answer sent to all clients") // Логируем успешную отправку.

	return nil // Возвращаем nil (ошибки нет).
} // Конец sendEverydayAnswerToAllClients.

// handleEverydayEvent обрабатывает события игры Everyday.
// Принимает событие игры и менеджер ботнета.
// Возвращает ошибку, если обработка не удалась.
func handleEverydayEvent(event *GameEvent, manager *BotnetManager) error { // Функция обработки событий Everyday.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	log.Printf("coordinator: handling Everyday event type=%s, eventID=%s", event.Type, event.EventID) // Логируем обработку события Everyday.

	var keysToProcess []string // Слайс для ключей счетчиков, которые нужно обработать.

	if event.Type == "audience/g-counter" { // Если это событие "audience/g-counter".
		// Извлекаем key из payload.
		if event.Payload == nil { // Если payload nil.
			return fmt.Errorf("payload is nil for Everyday event") // Возвращаем ошибку.
		} // Конец проверки payload.

		key, ok := event.Payload["key"].(string) // Получаем key из payload.
		if !ok || key == "" {                    // Если key не найден или пустой.
			return fmt.Errorf("key not found or empty in Everyday event payload") // Возвращаем ошибку.
		} // Конец проверки key.

		keysToProcess = []string{key} // Добавляем key в список для обработки.
	} else if event.Type == "client/welcome" { // Если это событие "client/welcome".
		// Извлекаем счетчики из entities.
		if event.Payload == nil { // Если payload nil.
			return fmt.Errorf("payload is nil for Everyday client/welcome event") // Возвращаем ошибку.
		} // Конец проверки payload.

		if result, ok := event.Payload["result"].(map[string]interface{}); ok { // Если есть result.
			if entities, ok := result["entities"].(map[string]interface{}); ok { // Если есть entities.
				for entityKey, entityValue := range entities { // Проходим по каждому ключу entity.
					// Проверяем, является ли ключ счетчиком типа cat_count_<NUMBER>.
					if strings.HasPrefix(entityKey, "cat_count_") { // Если ключ начинается с "cat_count_".
						// Проверяем, что значение - это массив с данными счетчика.
						if entityArray, ok := entityValue.([]interface{}); ok && len(entityArray) >= 2 { // Если это массив с минимум 2 элементами.
							if entityData, ok := entityArray[1].(map[string]interface{}); ok { // Если второй элемент - это map.
								if key, ok := entityData["key"].(string); ok && key != "" { // Если есть key.
									keysToProcess = append(keysToProcess, key) // Добавляем key в список для обработки.
								} // Конец проверки key.
							} // Конец проверки entityData.
						} // Конец проверки entityArray.
					} // Конец проверки префикса.
				} // Конец цикла по entities.
			} // Конец проверки entities.
		} // Конец проверки result.

		if len(keysToProcess) == 0 { // Если не найдено счетчиков.
			log.Printf("coordinator: Everyday client/welcome event has no cat_count_* counters, skipping") // Логируем отсутствие счетчиков.
			return nil                                                                                     // Возвращаем nil (ошибки нет).
		} // Конец проверки наличия счетчиков.
	} else { // Если это другое событие.
		log.Printf("coordinator: Everyday event type is not 'audience/g-counter' or 'client/welcome', skipping") // Логируем пропуск события.
		return nil                                                                                               // Возвращаем nil (ошибки нет).
	} // Конец проверки типа события.

	// Обрабатываем каждый найденный ключ.
	for _, key := range keysToProcess { // Проходим по каждому ключу.
		// Получаем текущий times и увеличиваем его для следующего раза.
		manager.mu.Lock()                     // Блокируем мьютекс для записи.
		currentTimes := manager.everydayTimes // Получаем текущий times.
		manager.everydayTimes++               // Увеличиваем times для следующего раза.
		manager.mu.Unlock()                   // Разблокируем мьютекс.

		log.Printf("coordinator: Everyday event key=%s, sending increment with times=%d", key, currentTimes) // Логируем отправку инкремента.

		// Отправляем команду всем клиентам для этого ключа.
		if err := sendEverydayAnswerToAllClients(event, key, currentTimes, manager); err != nil { // Отправляем ответ всем клиентам.
			return err // Возвращаем ошибку, если отправка не удалась.
		} // Конец проверки отправки.
	} // Конец цикла по ключам.

	return nil // Возвращаем nil (ошибки нет).
} // Конец handleEverydayEvent.

// handleQuiplash2Event обрабатывает события игры Quiplash 2.
// Принимает событие игры и менеджер ботнета.
// Возвращает ошибку, если обработка не удалась.
func handleQuiplash2Event(event *GameEvent, manager *BotnetManager) error { // Функция обработки событий Quiplash 2.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	log.Printf("coordinator: handling Quiplash 2 event type=%s, eventID=%s", event.Type, event.EventID) // Логируем обработку события Quiplash 2.

	// Пытаемся получить автоматический ответ из базы данных.
	answer, found := getAutoAnswer(event, manager.answerDB) // Получаем автоматический ответ.
	if found {                                              // Если ответ найден.
		log.Printf("coordinator: found auto-answer for event %s: %s", event.EventID, answer) // Логируем найденный автоматический ответ.
		// Отправляем команду всем клиентам.
		return sendAnswerToAllClients(event, answer, manager) // Отправляем ответ всем клиентам.
	} // Конец проверки наличия автоматического ответа.

	// Если автоматический ответ не найден, запрашиваем у пользователя.
	log.Printf("coordinator: no auto-answer found for event %s, prompting user", event.EventID) // Логируем отсутствие автоматического ответа.

	// Извлекаем информацию о вопросе из payload для отображения пользователю.
	questionText := extractQuestionText(event) // Извлекаем текст вопроса.

	// Запрашиваем ответ у пользователя.
	userAnswer, err := promptUserForAnswer(event, questionText) // Запрашиваем ответ у пользователя.
	if err != nil {                                             // Если запрос ответа не удался.
		return fmt.Errorf("failed to get user answer: %w", err) // Возвращаем ошибку.
	} // Конец проверки запроса ответа.

	if userAnswer == "" { // Если пользователь не ввёл ответ.
		log.Printf("coordinator: user did not provide answer, skipping") // Логируем пропуск ответа.
		return nil                                                       // Возвращаем nil (ошибки нет).
	} // Конец проверки пустого ответа.

	log.Printf("coordinator: user provided answer: %s", userAnswer) // Логируем ответ пользователя.

	// Отправляем ответ всем клиентам.
	return sendAnswerToAllClients(event, userAnswer, manager) // Отправляем ответ всем клиентам.
} // Конец handleQuiplash2Event.

// handleGenericEvent обрабатывает события для неизвестных игр.
// Принимает событие игры и менеджер ботнета.
// Возвращает ошибку, если обработка не удалась.
func handleGenericEvent(event *GameEvent, manager *BotnetManager) error { // Функция обработки общих событий.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	// Пытаемся получить автоматический ответ из базы данных.
	answer, found := getAutoAnswer(event, manager.answerDB) // Получаем автоматический ответ.
	if found {                                              // Если ответ найден.
		log.Printf("coordinator: found auto-answer for generic event %s: %s", event.EventID, answer) // Логируем найденный автоматический ответ.
		return sendAnswerToAllClients(event, answer, manager)                                        // Отправляем ответ всем клиентам.
	} // Конец проверки наличия автоматического ответа.

	// Если автоматический ответ не найден, запрашиваем у пользователя.
	questionText := extractQuestionText(event) // Извлекаем текст вопроса.

	userAnswer, err := promptUserForAnswer(event, questionText) // Запрашиваем ответ у пользователя.
	if err != nil {                                             // Если запрос ответа не удался.
		return fmt.Errorf("failed to get user answer: %w", err) // Возвращаем ошибку.
	} // Конец проверки запроса ответа.

	if userAnswer == "" { // Если пользователь не ввёл ответ.
		return nil // Возвращаем nil (ошибки нет).
	} // Конец проверки пустого ответа.

	return sendAnswerToAllClients(event, userAnswer, manager) // Отправляем ответ всем клиентам.
} // Конец handleGenericEvent.

// extractQuestionText извлекает текст вопроса из события.
// Принимает событие игры.
// Возвращает текст вопроса или пустую строку.
func extractQuestionText(event *GameEvent) string { // Функция извлечения текста вопроса.
	if event == nil || event.Payload == nil { // Если событие или payload nil.
		return "" // Возвращаем пустую строку.
	} // Конец проверки параметров.

	// Пытаемся извлечь текст вопроса из различных полей payload.
	if question, ok := event.Payload["question"].(string); ok { // Если поле question есть.
		return question // Возвращаем текст вопроса.
	} // Конец проверки поля question.

	if text, ok := event.Payload["text"].(string); ok { // Если поле text есть.
		return text // Возвращаем текст.
	} // Конец проверки поля text.

	if prompt, ok := event.Payload["prompt"].(string); ok { // Если поле prompt есть.
		return prompt // Возвращаем подсказку.
	} // Конец проверки поля prompt.

	return "" // Возвращаем пустую строку, если текст вопроса не найден.
} // Конец extractQuestionText.

// promptUserForAnswer запрашивает ответ у пользователя через CLI.
// Принимает событие игры и текст вопроса.
// Возвращает ответ пользователя или ошибку.
func promptUserForAnswer(event *GameEvent, questionText string) (string, error) { // Функция запроса ответа у пользователя.
	if event == nil { // Если событие nil.
		return "", fmt.Errorf("event is nil") // Возвращаем ошибку.
	} // Конец проверки события.

	// Выводим информацию о событии.
	fmt.Printf("\n=== Answer Required ===\n")   // Выводим заголовок.
	fmt.Printf("Game: %s\n", event.GameTag)     // Выводим тег игры.
	fmt.Printf("Event Type: %s\n", event.Type)  // Выводим тип события.
	fmt.Printf("Event ID: %s\n", event.EventID) // Выводим ID события.

	if questionText != "" { // Если текст вопроса есть.
		fmt.Printf("Question: %s\n", questionText) // Выводим текст вопроса.
	} // Конец проверки текста вопроса.

	fmt.Printf("Enter your answer (or press Enter to skip): ") // Выводим подсказку для ввода.

	// Читаем ввод пользователя.
	scanner := bufio.NewScanner(os.Stdin) // Создаём сканер для чтения из stdin.
	if !scanner.Scan() {                  // Если чтение не удалось.
		return "", fmt.Errorf("failed to read user input") // Возвращаем ошибку.
	} // Конец проверки чтения.

	answer := strings.TrimSpace(scanner.Text()) // Получаем введённый текст и убираем пробелы.

	if err := scanner.Err(); err != nil { // Если произошла ошибка сканера.
		return "", fmt.Errorf("scanner error: %w", err) // Возвращаем ошибку.
	} // Конец проверки ошибки сканера.

	return answer, nil // Возвращаем ответ пользователя и nil (ошибки нет).
} // Конец promptUserForAnswer.

// sendAnswerToAllClients отправляет команду ответа всем клиентам.
// Принимает событие игры, ответ и менеджер ботнета.
// Возвращает ошибку, если отправка не удалась.
func sendAnswerToAllClients(event *GameEvent, answer string, manager *BotnetManager) error { // Функция отправки ответа всем клиентам.
	if event == nil || manager == nil { // Если событие или менеджер nil.
		return fmt.Errorf("event or manager is nil") // Возвращаем ошибку.
	} // Конец проверки параметров.

	if answer == "" { // Если ответ пустой.
		return fmt.Errorf("answer is empty") // Возвращаем ошибку.
	} // Конец проверки ответа.

	// Создаём команду для клиентов.
	cmd := ClientCommand{ // Создаём команду.
		Type:    "answer",                     // Устанавливаем тип команды.
		EventID: event.EventID,                // Устанавливаем ID события.
		Answer:  answer,                       // Устанавливаем ответ.
		Payload: make(map[string]interface{}), // Инициализируем payload.
	} // Конец создания команды.

	// Добавляем дополнительную информацию в payload.
	cmd.Payload["gameTag"] = event.GameTag // Добавляем тег игры.
	cmd.Payload["eventType"] = event.Type  // Добавляем тип события.

	// Отправляем команду всем клиентам через канал.
	manager.mu.RLock()                  // Блокируем мьютекс для чтения.
	clientCount := len(manager.clients) // Получаем количество клиентов.
	manager.mu.RUnlock()                // Разблокируем мьютекс.

	log.Printf("coordinator: sending answer to %d clients", clientCount) // Логируем отправку ответа.

	// Отправляем команду в канал (все клиенты слушают этот канал).
	for i := 0; i < clientCount; i++ { // Проходим по количеству клиентов.
		select { // Выбираем между контекстом и отправкой команды.
		case <-manager.ctx.Done(): // Если контекст отменён.
			return fmt.Errorf("context canceled") // Возвращаем ошибку.
		case manager.commandChan <- cmd: // Если команда отправлена в канал.
			// Команда успешно отправлена.
		} // Конец select.
	} // Конец цикла отправки.

	log.Printf("coordinator: answer sent to all clients") // Логируем успешную отправку.

	return nil // Возвращаем nil (ошибки нет).
} // Конец sendAnswerToAllClients.
