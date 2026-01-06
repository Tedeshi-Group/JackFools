package commands // Пакет commands содержит реализацию CLI команд.

import ( // Начинаем блок импортов.
	"fmt"     // Печатаем сообщения пользователю.
	"log"     // Логируем события сервера/ошибки.
	"os"      // Читаем аргументы и завершаем процесс с кодом.
	"strings" // Работаем со строками для валидации.
	"unicode" // Проверяем, является ли символ буквой.
) // Закрываем блок импортов.

// Ddos реализует команду ddos.
// Принимает args — аргументы командной строки после подкоманды "ddos".
// Формат: ddos <code> [nickname]
// code — обязательный, 4 буквы (любой регистр).
// nickname — необязательный, 6 символов.
func Ddos(args []string) { // Реализация команды ddos.
	if len(args) < 1 { // Если код не указан.
		log.Printf("error: code is required") // Пишем в лог.
		os.Exit(2)                             // Выходим.
	} // Конец проверки наличия кода.

	code := args[0] // Берём первый аргумент как код.

	// Валидация обязательного аргумента code.
	if len(code) != 4 { // Проверяем длину кода (должно быть 4 символа).
		log.Printf("error: code must be exactly 4 characters, got %d", len(code)) // Пишем в лог.
		os.Exit(2) // Выходим.
	} // Конец проверки длины кода.

	// Проверяем, что все символы в code — буквы (любой регистр).
	for _, r := range code { // Проходим по каждому символу кода.
		if !unicode.IsLetter(r) { // Если символ не является буквой.
			log.Printf("error: code must contain only letters, got invalid character: %c", r) // Пишем в лог.
			os.Exit(2) // Выходим.
		} // Конец проверки символа.
	} // Конец цикла проверки символов.

	// Валидация необязательного аргумента nickname (если указан).
	var nickname string // Объявляем переменную для никнейма.
	if len(args) >= 2 { // Если указан второй аргумент.
		nickname = args[1] // Берём второй аргумент как никнейм.
		if len(nickname) != 6 { // Проверяем длину никнейма (должно быть 6 символов).
			log.Printf("error: nickname must be exactly 6 characters, got %d", len(nickname)) // Пишем в лог.
			os.Exit(2) // Выходим.
		} // Конец проверки длины никнейма.
	} // Конец проверки никнейма.

	// Здесь будет основная логика команды ddos.
	if nickname == "" { // Если никнейм не указан.
		log.Printf("ddos command: code=%s, nickname=(not set)", code) // Логируем параметры.
	} else { // Если никнейм указан.
		log.Printf("ddos command: code=%s, nickname=%s", code, nickname) // Логируем параметры.
	} // Конец проверки никнейма.

	fmt.Printf("Code: %s\n", strings.ToUpper(code)) // Выводим код в верхнем регистре.
	if nickname != "" { // Если никнейм указан.
		fmt.Printf("Nickname: %s\n", nickname) // Выводим никнейм.
	} // Конец проверки никнейма.
} // Конец Ddos.

