package main // Определяем главный пакет (точка входа программы).

import ( // Начинаем блок импортов.
	"fmt" // Печатаем сообщения пользователю.
	"log" // Логируем события сервера/ошибки.
	"os"  // Читаем аргументы и завершаем процесс с кодом.

	"jackfools/client/internal/commands" // Команды CLI.
) // Закрываем блок импортов.

func main() { // Точка входа.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds) // Делаем логи более информативными.

	if len(os.Args) < 2 { // Если команда не указана.
		printUsage() // Печатаем подсказку.
		os.Exit(2)   // Выходим с кодом ошибки аргументов.
	} // Конец проверки команды.

	switch os.Args[1] { // Смотрим подкоманду.
	case "serve": // Запуск локального API.
		commands.Serve(os.Args[2:]) // Передаём оставшиеся аргументы в команду serve.
	case "ddos": // Команда ddos.
		commands.Ddos(os.Args[2:]) // Передаём оставшиеся аргументы в команду ddos.
	default: // Неизвестная команда.
		printUsage() // Печатаем подсказку.
		os.Exit(2)   // Выходим с кодом ошибки.
	} // Конец switch.
} // Конец main.

func printUsage() { // Печать справки.
	fmt.Fprintln(os.Stderr, "Usage: jackfools <command> [options]")   // Краткая инструкция.
	fmt.Fprintln(os.Stderr, "Commands:")                              // Список команд.
	fmt.Fprintln(os.Stderr, "  serve [--port 27124] [--token <hex>]") // Команда serve.
	fmt.Fprintln(os.Stderr, "  ddos <code> [nickname]")               // Команда ddos.
} // Конец printUsage.
