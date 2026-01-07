package commands // Пакет commands содержит реализацию CLI команд.

import ( // Начинаем блок импортов.
	"context"       // Отменяем запросы и горутины при завершении.
	"crypto/rand"   // Генерируем криптографически стойкие случайные числа для имён зрителей.
	"encoding/json" // Парсим JSON сообщения от WebSocket.
	"fmt"           // Печатаем сообщения пользователю.
	"log"           // Логируем события сервера/ошибки.
	"net/http"      // Используем http.Header для WebSocket заголовков.
	"net/url"       // Работаем с URL параметрами.
	"os"            // Читаем аргументы и завершаем процесс с кодом.
	"os/signal"     // Обрабатываем сигналы для graceful shutdown.
	"strconv"       // Преобразуем строки в числа.
	"strings"       // Работаем со строками для валидации.
	"sync"          // Синхронизируем горутины.
	"syscall"       // Используем syscall.SIGINT, syscall.SIGTERM для обработки сигналов.
	"time"          // Устанавливаем таймауты для запросов.
	"unicode"       // Проверяем, является ли символ буквой.

	"github.com/google/uuid"       // Генерируем UUID для user-id.
	"github.com/gorilla/websocket" // Работаем с WebSocket подключениями.
) // Закрываем блок импортов.

// BotnetManager управляет всеми подключениями ботнета.
// Координирует работу координатора и клиентов.
type BotnetManager struct { // Структура менеджера ботнета.
	mu            sync.RWMutex            // Мьютекс для защиты общих данных.
	coordinator   *websocket.Conn         // WebSocket соединение координатора.
	clients       map[int]*websocket.Conn // Карта клиентов по их ID.
	ctx           context.Context         // Контекст для отмены операций.
	cancel        context.CancelFunc      // Функция отмены контекста.
	answerDB      *AnswerDatabase         // База правильных ответов (обычные вопросы).
	finalRoundDB  *AnswerDatabase         // База правильных ответов для финального раунда.
	commandChan   chan ClientCommand      // Канал для отправки команд клиентам.
	gameTag       string                  // Кешированный тег игры (извлекается из первого сообщения).
	everydayTimes int                     // Счётчик times для игры Everyday (начинается с 50, увеличивается).
} // Конец BotnetManager.

// ClientCommand представляет команду, которую координатор отправляет клиентам.
type ClientCommand struct { // Структура команды клиенту.
	Type    string                 // Тип команды (например, "answer").
	EventID string                 // ID события, на которое нужно ответить.
	Answer  string                 // Ответ для отправки.
	Payload map[string]interface{} // Дополнительные данные команды.
} // Конец ClientCommand.

// Botnet реализует команду botnet.
// Принимает args — аргументы командной строки после подкоманды "botnet".
// Формат: botnet <code> [num_clients]
// code — обязательный, 4 буквы (любой регистр).
// num_clients — необязательный, количество клиентов (по умолчанию 10).
func Botnet(args []string) { // Реализация команды botnet.
	if len(args) < 1 { // Если код не указан.
		log.Printf("error: code is required") // Пишем в лог.
		os.Exit(2)                            // Выходим.
	} // Конец проверки наличия кода.

	code := args[0] // Берём первый аргумент как код.

	// Валидация обязательного аргумента code.
	if len(code) != 4 { // Проверяем длину кода (должно быть 4 символа).
		log.Printf("error: code must be exactly 4 characters, got %d", len(code)) // Пишем в лог.
		os.Exit(2)                                                                // Выходим.
	} // Конец проверки длины кода.

	// Проверяем, что все символы в code — буквы (любой регистр).
	for _, r := range code { // Проходим по каждому символу кода.
		if !unicode.IsLetter(r) { // Если символ не является буквой.
			log.Printf("error: code must contain only letters, got invalid character: %c", r) // Пишем в лог.
			os.Exit(2)                                                                        // Выходим.
		} // Конец проверки символа.
	} // Конец цикла проверки символов.

	// Валидация необязательного аргумента num_clients.
	numClients := 10    // Значение по умолчанию.
	if len(args) >= 2 { // Если указан второй аргумент.
		var err error                           // Переменная для ошибки.
		numClients, err = strconv.Atoi(args[1]) // Преобразуем строку в число.
		if err != nil {                         // Если преобразование не удалось.
			log.Printf("error: num_clients must be a number, got: %s", args[1]) // Пишем в лог.
			os.Exit(2)                                                          // Выходим.
		} // Конец проверки преобразования.
		if numClients < 1 { // Если количество клиентов меньше 1.
			log.Printf("error: num_clients must be at least 1, got: %d", numClients) // Пишем в лог.
			os.Exit(2)                                                               // Выходим.
		} // Конец проверки минимального значения.
		if numClients > 100 { // Если количество клиентов больше 100.
			log.Printf("error: num_clients must be at most 100, got: %d", numClients) // Пишем в лог.
			os.Exit(2)                                                                // Выходим.
		} // Конец проверки максимального значения.
	} // Конец проверки num_clients.

	// Загружаем список хостов из JSON файла.
	hostsURL := "https://gist.githubusercontent.com/Geardung/67d9695f4f09836364cbe724721d3046/raw/8a72727071a08134eb212a2f5f5df00792107c22/hosts.json" // URL для загрузки списка хостов.
	hosts, err := fetchHosts(hostsURL)                                                                                                                 // Загружаем хосты.
	if err != nil {                                                                                                                                    // Если не удалось загрузить хосты.
		log.Printf("error: failed to fetch hosts: %v", err) // Логируем ошибку.
		os.Exit(2)                                          // Выходим с кодом ошибки.
	} // Конец проверки загрузки хостов.

	if len(hosts) == 0 { // Если список хостов пуст.
		log.Printf("error: no hosts found") // Логируем ошибку.
		os.Exit(2)                          // Выходим с кодом ошибки.
	} // Конец проверки списка хостов.

	log.Printf("checking code %s on %d hosts", strings.ToUpper(code), len(hosts)) // Логируем начало проверки.

	// Проверяем все хосты параллельно и находим актуальный.
	activeHost, roomInfo, err := findActiveHost(hosts, strings.ToUpper(code)) // Ищем актуальный хост и получаем информацию о комнате.
	if err != nil {                                                           // Если не удалось найти актуальный хост.
		log.Printf("error: %v", err) // Логируем ошибку.
		os.Exit(2)                   // Выходим с кодом ошибки.
	} // Конец проверки результата поиска.

	log.Printf("active host found: %s", activeHost) // Логируем найденный хост.
	if roomInfo == nil {                            // Если информация о комнате не получена.
		log.Printf("error: room info is required") // Логируем ошибку.
		os.Exit(2)                                 // Выходим с кодом ошибки.
	} // Конец проверки информации о комнате.

	if roomInfo.AudienceHost == "" { // Если хост для аудитории не указан.
		log.Printf("error: audience host is not available for this room") // Логируем ошибку.
		os.Exit(2)                                                        // Выходим с кодом ошибки.
	} // Конец проверки хоста аудитории.

	log.Printf("room info: appTag=%s, audienceHost=%s, maxPlayers=%d", roomInfo.AppTag, roomInfo.AudienceHost, roomInfo.MaxPlayers) // Логируем информацию о комнате.

	// Создаём контекст с возможностью отмены.
	ctx, cancel := context.WithCancel(context.Background()) // Создаём контекст с отменой.
	defer cancel()                                          // Отменяем контекст при выходе из функции.

	// Обрабатываем сигналы для graceful shutdown.
	sigChan := make(chan os.Signal, 1)                      // Создаём канал для сигналов.
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM) // Подписываемся на SIGINT и SIGTERM.

	// Запускаем горутину для обработки сигналов.
	go func() { // Запускаем горутину для обработки сигналов.
		<-sigChan                                                      // Ждём сигнал.
		log.Printf("received shutdown signal, closing connections...") // Логируем получение сигнала.
		cancel()                                                       // Отменяем контекст, что приведёт к закрытию всех подключений.
	}() // Запускаем горутину.

	// Загружаем базу ответов онлайн (обычные вопросы).
	answersURL := "https://gist.githubusercontent.com/Geardung/67d9695f4f09836364cbe724721d3046/raw/efdfff41ea60cb6b93cc03cfad09ededefd22f4e/triviadeath2-tjsp-questions.json" // URL для загрузки базы ответов для Trivia Death 2.
	answerDB, err := loadAnswerDatabase(answersURL)                                                                                                                            // Загружаем базу ответов.
	if err != nil {                                                                                                                                                            // Если не удалось загрузить базу ответов.
		log.Printf("warning: failed to load answer database: %v, continuing without auto-answers", err) // Логируем предупреждение.
		answerDB = &AnswerDatabase{}                                                                    // Создаём пустую базу ответов.
	} // Конец проверки загрузки базы ответов.

	// Загружаем базу ответов для финального раунда.
	finalRoundURL := "https://gist.githubusercontent.com/Geardung/67d9695f4f09836364cbe724721d3046/raw/ea10557f20c3cb9662ed8842892f163129574f74/triviadeath2-tjsp-final.json" // URL для загрузки базы ответов финального раунда.
	finalRoundDB, err := loadFinalRoundDatabase(finalRoundURL)                                                                                                                // Загружаем базу ответов финального раунда.
	if err != nil {                                                                                                                                                           // Если не удалось загрузить базу ответов финального раунда.
		log.Printf("warning: failed to load final round database: %v, continuing without final round auto-answers", err) // Логируем предупреждение.
		finalRoundDB = &AnswerDatabase{                                                                                  // Создаём пустую базу ответов.
			FinalRoundQuestions: make(map[string][]string), // Инициализируем карту вопросов финального раунда.
		} // Конец создания базы.
	} // Конец проверки загрузки базы ответов финального раунда.

	// Создаём менеджер ботнета.
	manager := &BotnetManager{ // Создаём менеджер.
		clients:       make(map[int]*websocket.Conn), // Инициализируем карту клиентов.
		ctx:           ctx,                           // Устанавливаем контекст.
		cancel:        cancel,                        // Устанавливаем функцию отмены.
		answerDB:      answerDB,                      // Устанавливаем базу ответов (обычные вопросы).
		finalRoundDB:  finalRoundDB,                  // Устанавливаем базу ответов для финального раунда.
		commandChan:   make(chan ClientCommand, 100), // Создаём канал для команд (буфер 100).
		everydayTimes: 50,                            // Инициализируем счётчик times для Everyday (начинаем с 50).
		gameTag:       roomInfo.AppTag,               // Устанавливаем тег игры из информации о комнате (если доступен).
	} // Конец создания менеджера.

	// Подключаем координатора.
	coordinatorID := uuid.New().String()                                                                  // Генерируем UUID для координатора.
	coordConn, err := connectAsAudience(ctx, roomInfo.AudienceHost, strings.ToUpper(code), coordinatorID) // Подключаемся как зритель.
	if err != nil {                                                                                       // Если подключение не удалось.
		log.Printf("error: failed to connect coordinator: %v", err) // Логируем ошибку.
		os.Exit(2)                                                  // Выходим с кодом ошибки.
	} // Конец проверки подключения координатора.

	manager.coordinator = coordConn                  // Сохраняем соединение координатора.
	log.Printf("coordinator connected successfully") // Логируем успешное подключение координатора.

	// Запускаем цикл координатора в отдельной горутине.
	go runCoordinator(coordConn, strings.ToUpper(code), manager) // Запускаем координатор.

	// Подключаем клиентов.
	var wg sync.WaitGroup             // Создаём WaitGroup для синхронизации горутин.
	for i := 0; i < numClients; i++ { // Проходим по количеству клиентов.
		wg.Add(1) // Увеличиваем счётчик WaitGroup.

		go func(clientID int) { // Запускаем горутину для каждого клиента.
			defer wg.Done() // Уменьшаем счётчик WaitGroup при выходе из горутины.

			clientUUID := uuid.New().String()                                                                   // Генерируем UUID для клиента.
			clientConn, err := connectAsAudience(ctx, roomInfo.AudienceHost, strings.ToUpper(code), clientUUID) // Подключаемся как зритель.
			if err != nil {                                                                                     // Если подключение не удалось.
				log.Printf("error: failed to connect client %d: %v", clientID, err) // Логируем ошибку.
				return                                                              // Выходим из горутины.
			} // Конец проверки подключения.

			manager.mu.Lock()                      // Блокируем мьютекс.
			manager.clients[clientID] = clientConn // Сохраняем соединение клиента.
			manager.mu.Unlock()                    // Разблокируем мьютекс.

			log.Printf("client %d connected successfully", clientID) // Логируем успешное подключение клиента.

			// Запускаем цикл клиента.
			runClient(clientConn, clientID, manager) // Запускаем клиента.
		}(i) // Передаём ID клиента в горутину.
	} // Конец цикла подключения клиентов.

	// Выводим информацию пользователю.
	fmt.Printf("Botnet started:\n")                            // Выводим заголовок.
	fmt.Printf("  Code: %s\n", strings.ToUpper(code))          // Выводим код.
	fmt.Printf("  Active host: %s\n", activeHost)              // Выводим найденный хост.
	fmt.Printf("  Audience host: %s\n", roomInfo.AudienceHost) // Выводим хост аудитории.
	fmt.Printf("  App tag: %s\n", roomInfo.AppTag)             // Выводим тег приложения.
	fmt.Printf("  Clients: %d\n", numClients)                  // Выводим количество клиентов.
	fmt.Printf("  Press Ctrl+C to stop\n")                     // Выводим подсказку.

	// Ждём завершения всех горутин или отмены контекста.
	wg.Wait() // Ждём завершения всех горутин клиентов.

	log.Printf("botnet stopped") // Логируем остановку ботнета.
} // Конец Botnet.

// connectAsAudience создаёт WebSocket подключение к комнате игры как зритель.
// Принимает контекст, домен хоста аудитории, код комнаты и UUID пользователя.
// Возвращает WebSocket соединение или ошибку.
func connectAsAudience(ctx context.Context, audienceHost, code, userID string) (*websocket.Conn, error) { // Функция подключения как зритель.
	// Генерируем случайное имя для зрителя (4 символа).
	audienceName := generateRandomAudienceName() // Генерируем случайное имя.

	// Формируем URL с параметрами для зрителя.
	// Правильный путь для зрителей: /api/v2/audience/{code}/play
	wsURL := url.URL{ // Создаём структуру URL.
		Scheme:   "wss",                                                                                              // Используем протокол wss (WebSocket Secure).
		Host:     audienceHost,                                                                                       // Устанавливаем хост аудитории.
		Path:     fmt.Sprintf("/api/v2/audience/%s/play", code),                                                      // Устанавливаем правильный путь для зрителей.
		RawQuery: fmt.Sprintf("role=audience&name=%s&format=json&user-id=%s", url.QueryEscape(audienceName), userID), // Устанавливаем параметры запроса (role=audience, name обязателен, userID не требует экранирования, так как это UUID).
	} // Конец создания URL.

	log.Printf("debug: connecting WebSocket as audience to %s", wsURL.String()) // Логируем URL подключения.

	// Создаём заголовки для WebSocket handshake.
	header := make(http.Header)                                                                                                                                     // Создаём карту заголовков.
	header.Set("Host", audienceHost)                                                                                                                                // Устанавливаем заголовок Host.
	header.Set("Pragma", "no-cache")                                                                                                                                // Устанавливаем заголовок Pragma.
	header.Set("Cache-Control", "no-cache")                                                                                                                         // Устанавливаем заголовок Cache-Control.
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 YaBrowser/25.12.0.0 Safari/537.36") // Устанавливаем заголовок User-Agent.
	header.Set("Origin", "https://jackbox.fun")                                                                                                                     // Устанавливаем заголовок Origin.
	header.Set("Accept-Encoding", "gzip, deflate, br, zstd")                                                                                                        // Устанавливаем заголовок Accept-Encoding.
	header.Set("Accept-Language", "ru,en;q=0.9")                                                                                                                    // Устанавливаем заголовок Accept-Language.

	// Создаём WebSocket dialer.
	dialer := websocket.Dialer{ // Создаём структуру Dialer для подключения.
		HandshakeTimeout:  10 * time.Second,     // Устанавливаем таймаут handshake в 10 секунд.
		EnableCompression: true,                 // Включаем сжатие (permessage-deflate).
		Subprotocols:      []string{"ecast-v0"}, // Устанавливаем подпротокол ecast-v0.
	} // Конец создания Dialer.

	// Подключаемся к WebSocket серверу.
	// Используем Dial вместо DialContext, так как контекст нужен только для отмены,
	// а мы проверяем его отдельно перед подключением.
	conn, resp, err := dialer.Dial(wsURL.String(), header) // Выполняем подключение с заголовками.
	if err != nil {                                        // Если подключение не удалось.
		if resp != nil { // Если ответ получен.
			log.Printf("debug: WebSocket connection failed: %v (status: %d)", err, resp.StatusCode) // Логируем ошибку с статусом.
		} else { // Если ответа нет.
			log.Printf("debug: WebSocket connection failed: %v", err) // Логируем ошибку без статуса.
		} // Конец проверки ответа.
		return nil, fmt.Errorf("failed to connect WebSocket: %w", err) // Возвращаем ошибку.
	} // Конец проверки подключения.

	// Проверяем, не отменён ли контекст после подключения.
	select { // Выбираем между контекстом.
	case <-ctx.Done(): // Если контекст отменён.
		conn.Close()                                                             // Закрываем соединение.
		return nil, fmt.Errorf("context canceled before connection established") // Возвращаем ошибку.
	default: // Если контекст не отменён, продолжаем.
	} // Конец select.

	log.Printf("debug: WebSocket connected successfully as audience (user-id: %s)", userID) // Логируем успешное подключение.

	return conn, nil // Возвращаем соединение и nil (ошибки нет).
} // Конец connectAsAudience.

// runCoordinator запускает главный цикл координатора.
// Читает события от сервера и управляет клиентами.
// Принимает WebSocket соединение, код комнаты и менеджер ботнета.
func runCoordinator(conn *websocket.Conn, code string, manager *BotnetManager) { // Функция запуска координатора.
	defer conn.Close() // Закрываем соединение при выходе из функции.

	for { // Бесконечный цикл чтения событий.
		// Проверяем, не отменён ли контекст.
		select { // Выбираем между контекстом и чтением сообщения.
		case <-manager.ctx.Done(): // Если контекст отменён.
			log.Printf("coordinator: context canceled, shutting down") // Логируем отмену контекста.
			return                                                     // Выходим из цикла.
		default: // Если контекст не отменён, продолжаем.
		} // Конец select.

		// Отключаем таймаут чтения (устанавливаем в нулевое время для отключения).
		conn.SetReadDeadline(time.Time{}) // Отключаем таймаут чтения.

		// Читаем сообщение от сервера.
		_, message, err := conn.ReadMessage() // Читаем сообщение.
		if err != nil {                       // Если произошла ошибка чтения.
			// Проверяем, не была ли это ошибка отмены контекста.
			if manager.ctx.Err() != nil { // Если контекст отменён.
				log.Printf("coordinator: context canceled") // Логируем отмену контекста.
				return                                      // Выходим из цикла.
			} // Конец проверки контекста.
			// Проверяем, не закрыто ли соединение.
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) { // Если это неожиданное закрытие.
				log.Printf("coordinator: WebSocket read error: %v", err) // Логируем ошибку.
				return                                                   // Выходим из цикла.
			} // Конец проверки ошибки.
			// Если это другая ошибка, логируем и выходим.
			log.Printf("coordinator: WebSocket read error: %v", err) // Логируем ошибку.
			return                                                   // Выходим из цикла.
		} // Конец проверки ошибки.

		// Парсим и обрабатываем сообщение.
		msg, err := parseWebSocketMessage(message) // Парсим сообщение.
		if err != nil {                            // Если парсинг не удался.
			log.Printf("coordinator: failed to parse message: %v", err) // Логируем ошибку парсинга.
			continue                                                    // Продолжаем цикл.
		} // Конец проверки парсинга.

		// Преобразуем WebSocket сообщение в GameEvent.
		event, err := parseGameEvent(msg) // Преобразуем сообщение в событие.
		if err != nil {                   // Если преобразование не удалось.
			log.Printf("coordinator: failed to parse game event: %v", err) // Логируем ошибку преобразования.
			continue                                                       // Продолжаем цикл.
		} // Конец проверки преобразования.

		// Если gameTag извлечён из события и ещё не кеширован, сохраняем его.
		if event.GameTag != "" && manager.gameTag == "" { // Если тег игры найден и ещё не кеширован.
			manager.mu.Lock()                                             // Блокируем мьютекс.
			manager.gameTag = event.GameTag                               // Сохраняем тег игры.
			manager.mu.Unlock()                                           // Разблокируем мьютекс.
			log.Printf("coordinator: cached game tag: %s", event.GameTag) // Логируем кеширование тега игры.
		} // Конец проверки кеширования.

		// Если gameTag не найден в событии, но есть в кеше, используем кешированный.
		if event.GameTag == "" && manager.gameTag != "" { // Если тег игры не найден, но есть в кеше.
			event.GameTag = manager.gameTag // Используем кешированный тег.
			// Переопределяем requiresAnswer после установки gameTag из кеша.
			event.RequiresAnswer = shouldRequireAnswer(event) // Переопределяем необходимость ответа.
		} // Конец проверки использования кеша.

		// Обрабатываем событие.
		if err := handleEvent(event, manager); err != nil { // Обрабатываем событие.
			log.Printf("coordinator: failed to handle event: %v", err) // Логируем ошибку обработки.
			// Продолжаем работу, даже если обработка не удалась.
		} // Конец проверки обработки.
	} // Конец бесконечного цикла.
} // Конец runCoordinator.

// runClient запускает цикл клиента.
// Клиент слушает команды от координатора и отправляет ответы.
// Принимает WebSocket соединение, ID клиента и менеджер ботнета.
func runClient(conn *websocket.Conn, clientID int, manager *BotnetManager) { // Функция запуска клиента.
	defer conn.Close() // Закрываем соединение при выходе из функции.

	// Запускаем горутину для чтения сообщений от сервера (чтобы поддерживать соединение).
	go func() { // Запускаем горутину для чтения сообщений.
		for { // Бесконечный цикл чтения сообщений.
			// Проверяем, не отменён ли контекст.
			select { // Выбираем между контекстом и чтением сообщения.
			case <-manager.ctx.Done(): // Если контекст отменён.
				return // Выходим из цикла.
			default: // Если контекст не отменён, продолжаем.
			} // Конец select.

			// Отключаем таймаут чтения (устанавливаем в нулевое время для отключения).
			conn.SetReadDeadline(time.Time{}) // Отключаем таймаут чтения.

			// Читаем сообщение от сервера.
			_, message, err := conn.ReadMessage() // Читаем сообщение.
			if err != nil {                       // Если произошла ошибка чтения.
				if manager.ctx.Err() != nil { // Если контекст отменён.
					return // Выходим из цикла.
				} // Конец проверки контекста.
				// Проверяем, не закрыто ли соединение.
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) { // Если это неожиданное закрытие.
					log.Printf("client %d: WebSocket read error: %v", clientID, err) // Логируем ошибку.
					return                                                           // Выходим из цикла.
				} // Конец проверки ошибки.
				// Если это другая ошибка, логируем и выходим.
				log.Printf("client %d: WebSocket read error: %v", clientID, err) // Логируем ошибку.
				return                                                           // Выходим из цикла.
			} // Конец проверки ошибки.

			// Игнорируем сообщения от сервера (координатор обрабатывает их).
			_ = message // Игнорируем содержимое сообщения.
		} // Конец бесконечного цикла.
	}() // Запускаем горутину.

	// Слушаем команды от координатора.
	for { // Бесконечный цикл ожидания команд.
		// Проверяем, не отменён ли контекст.
		select { // Выбираем между контекстом и командой.
		case <-manager.ctx.Done(): // Если контекст отменён.
			log.Printf("client %d: context canceled, shutting down", clientID) // Логируем отмену контекста.
			return                                                             // Выходим из цикла.
		case cmd := <-manager.commandChan: // Если получена команда от координатора.
			// Отправляем ответ на команду.
			if err := sendClientResponse(conn, cmd); err != nil { // Отправляем ответ.
				log.Printf("client %d: failed to send response: %v", clientID, err) // Логируем ошибку отправки.
			} else { // Если отправка успешна.
				log.Printf("client %d: sent response for event %s", clientID, cmd.EventID) // Логируем успешную отправку.
			} // Конец проверки отправки.
		} // Конец select.
	} // Конец бесконечного цикла.
} // Конец runClient.

// sendClientResponse отправляет ответ от клиента на сервер.
// Принимает WebSocket соединение и команду от координатора.
// Возвращает ошибку, если отправка не удалась.
func sendClientResponse(conn *websocket.Conn, cmd ClientCommand) error { // Функция отправки ответа клиента.
	var message map[string]interface{} // Переменная для сообщения.

	// Проверяем, является ли это ответом для Everyday.
	if gameTag, ok := cmd.Payload["gameTag"].(string); ok && gameTag == "everyday" { // Если это Everyday.
		// Получаем opcode, key и times из payload.
		opcode, _ := cmd.Payload["opcode"].(string) // Получаем opcode.
		key, _ := cmd.Payload["key"].(string)       // Получаем key.
		times, _ := cmd.Payload["times"].(int)      // Получаем times.

		// Формируем сообщение в формате Everyday.
		message = map[string]interface{}{ // Создаём карту для сообщения.
			"seq":    1,      // Порядковый номер сообщения (начинаем с 1).
			"opcode": opcode, // Код операции для Everyday.
			"params": map[string]interface{}{ // Параметры сообщения.
				"key":   key,   // Key из события.
				"times": times, // Times (начинается с 50, увеличивается).
			}, // Конец параметров.
		} // Конец создания сообщения.
	} else if gameTag, ok := cmd.Payload["gameTag"].(string); ok && (gameTag == "triviadeath2-tjsp" || strings.Contains(gameTag, "triviadeath2")) { // Если это Trivia Death 2.
		// Проверяем, является ли это финальным раундом.
		isFinalRound, _ := cmd.Payload["isFinalRound"].(bool) // Получаем флаг финального раунда.
		if isFinalRound {                                     // Если это финальный раунд.
			// Для финального раунда используем строку с индексами через запятую (например, "1,2").
			voteString := cmd.Answer // Используем строку ответа напрямую (уже содержит индексы через запятую).

			// Формируем сообщение в формате Trivia Death 2 для финального раунда.
			message = map[string]interface{}{ // Создаём карту для сообщения.
				"seq":    1,                                // Порядковый номер сообщения (начинаем с 1).
				"opcode": "audience/count-group/increment", // Код операции для подтверждения ответа зрителя.
				"params": map[string]interface{}{ // Параметры сообщения.
					"name":  "TriviaDeath2AudienceChoice", // Имя группы подсчёта.
					"vote":  voteString,                   // Индексы выбранных ответов через запятую (например, "1,2").
					"times": 1,                            // Количество раз (всегда 1).
				}, // Конец параметров.
			} // Конец создания сообщения.
		} else { // Если это обычный раунд.
			// Для обычного раунда используем один индекс.
			answerIndex, ok := cmd.Payload["answerIndex"].(int) // Получаем индекс ответа.
			if !ok {                                            // Если индекс не найден.
				// Пытаемся преобразовать из строки.
				if answerStr := cmd.Answer; answerStr != "" { // Если есть строка ответа.
					if idx, err := strconv.Atoi(answerStr); err == nil { // Если удалось преобразовать в число.
						answerIndex = idx // Устанавливаем индекс.
					} else { // Если преобразование не удалось.
						return fmt.Errorf("invalid answer index for Trivia Death 2: %s", answerStr) // Возвращаем ошибку.
					} // Конец проверки преобразования.
				} else { // Если строки ответа нет.
					return fmt.Errorf("answer index not found for Trivia Death 2") // Возвращаем ошибку.
				} // Конец проверки строки ответа.
			} // Конец проверки индекса.

			// Формируем сообщение в формате Trivia Death 2 для обычного раунда.
			message = map[string]interface{}{ // Создаём карту для сообщения.
				"seq":    1,                                // Порядковый номер сообщения (начинаем с 1).
				"opcode": "audience/count-group/increment", // Код операции для подтверждения ответа зрителя.
				"params": map[string]interface{}{ // Параметры сообщения.
					"name":  "TriviaDeath2AudienceChoice",   // Имя группы подсчёта.
					"vote":  fmt.Sprintf("%d", answerIndex), // Индекс выбранного ответа (в виде строки).
					"times": 1,                              // Количество раз (всегда 1).
				}, // Конец параметров.
			} // Конец создания сообщения.
		} // Конец проверки финального раунда.
	} else if gameTag, ok := cmd.Payload["gameTag"].(string); ok && gameTag == "pollposition" { // Если это Poll Position.
		// Получаем vote, opcode и name из payload.
		vote, _ := cmd.Payload["vote"].(string)     // Получаем vote ("0" или "1").
		opcode, _ := cmd.Payload["opcode"].(string) // Получаем opcode.
		name, _ := cmd.Payload["name"].(string)     // Получаем name группы подсчёта.

		// Если vote не найден в payload, используем Answer.
		if vote == "" { // Если vote не найден.
			vote = cmd.Answer // Используем Answer как vote.
		} // Конец проверки vote.

		// Если opcode не найден, используем значение по умолчанию.
		if opcode == "" { // Если opcode не найден.
			opcode = "audience/count-group/increment" // Устанавливаем opcode по умолчанию.
		} // Конец проверки opcode.

		// Если name не найден, используем значение по умолчанию.
		if name == "" { // Если name не найден.
			name = "Poll Position Vote" // Устанавливаем name по умолчанию.
		} // Конец проверки name.

		// Формируем сообщение в формате Poll Position.
		message = map[string]interface{}{ // Создаём карту для сообщения.
			"seq":    1,      // Порядковый номер сообщения (начинаем с 1).
			"opcode": opcode, // Код операции для Poll Position.
			"params": map[string]interface{}{ // Параметры сообщения.
				"name":  name, // Имя группы подсчёта.
				"vote":  vote, // Vote ("0" или "1").
				"times": 1,    // Количество раз (всегда 1).
			}, // Конец параметров.
		} // Конец создания сообщения.
	} else { // Если это не Trivia Death 2 и не Poll Position, используем общий формат.
		// Формируем JSON сообщение для отправки.
		message = map[string]interface{}{ // Создаём карту для сообщения.
			"type":    cmd.Type,    // Устанавливаем тип команды.
			"eventId": cmd.EventID, // Устанавливаем ID события.
			"answer":  cmd.Answer,  // Устанавливаем ответ.
		} // Конец создания сообщения.

		// Добавляем дополнительные данные из payload, если они есть.
		for k, v := range cmd.Payload { // Проходим по дополнительным данным.
			message[k] = v // Добавляем данные в сообщение.
		} // Конец цикла.
	} // Конец проверки типа игры.

	// Кодируем сообщение в JSON.
	data, err := json.Marshal(message) // Кодируем сообщение в JSON.
	if err != nil {                    // Если кодирование не удалось.
		return fmt.Errorf("failed to marshal response: %w", err) // Возвращаем ошибку.
	} // Конец проверки кодирования.

	// Отправляем сообщение через WebSocket.
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil { // Отправляем сообщение.
		return fmt.Errorf("failed to write message: %w", err) // Возвращаем ошибку.
	} // Конец проверки отправки.

	return nil // Возвращаем nil (ошибки нет).
} // Конец sendClientResponse.

// generateRandomAudienceName генерирует случайное имя для зрителя.
// Возвращает строку из 4 заглавных букв (например, "AUDI").
func generateRandomAudienceName() string { // Функция генерации случайного имени зрителя.
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" // Набор заглавных букв для генерации имени.
	const nameLength = 4                         // Длина имени (4 символа).

	b := make([]byte, nameLength) // Создаём слайс байт для случайных символов.
	rand.Read(b)                  // Заполняем слайс случайными байтами.

	result := make([]byte, nameLength) // Создаём слайс для результата.
	for i := range b {                 // Проходим по каждому байту.
		result[i] = charset[b[i]%byte(len(charset))] // Преобразуем байт в символ из charset.
	} // Конец цикла.

	return string(result) // Возвращаем строку из 4 заглавных букв.
} // Конец generateRandomAudienceName.
