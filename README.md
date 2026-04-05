# stalcraft-wrapper

[![en](https://img.shields.io/badge/lang-English-blue)](README.en.md)

JVM-враппер для STALCRAFT, который динамически подбирает JVM-флаги и повышает приоритет процесса под ваше железо.

## Что делает

- **Определяет** систему: RAM (всего + свободно), ядра CPU, поддержка large pages
- **Генерирует** оптимальные JVM-флаги: размер heap, потоки GC, G1 region size, metaspace, code cache и другое
- **Заменяет** стандартные JVM-аргументы лаунчера на оптимизированные
- **Бустит** процесс игры: `HIGH_PRIORITY_CLASS`, приоритет памяти, приоритет I/O
- **Устанавливается** прозрачно через Windows IFEO — файлы игры не затрагиваются

## Установка

Скачайте `wrapper.exe` из [Releases](../../releases), положите куда угодно и запустите от админа:

```
wrapper.exe --install
```

Готово. Каждый запуск `stalcraft.exe` теперь автоматически проходит через враппер.

### Другие команды

```
wrapper.exe --status      # проверить статус установки
wrapper.exe --uninstall   # удалить IFEO-хук
```

## Как это работает

Windows [Image File Execution Options](https://learn.microsoft.com/en-us/previous-versions/windows/desktop/xperf/image-file-execution-options) перехватывает запуск `stalcraft.exe` и перенаправляет его через враппер. Враппер:

1. Определяет железо через `GlobalMemoryStatusEx`, `runtime.NumCPU`, `GetLargePageMinimum`
2. Рассчитывает оптимальные JVM-флаги на основе доступных ресурсов
3. Убирает конфликтующие флаги из оригинальных аргументов лаунчера
4. Запускает настоящий `stalcraft.exe` с подобранными флагами и `HIGH_PRIORITY_CLASS`
5. Применяет пост-буст: отключает снижение приоритета, выставляет максимальный приоритет памяти и I/O

### Динамический подбор

| Параметр | Формула |
|----------|---------|
| Heap | 50% свободной RAM, floor 25% от общей, cap min(16g, 75% от общей) |
| ParallelGCThreads | ядра - 2, минимум 2 |
| ConcGCThreads | parallel / 4, минимум 1 |
| G1HeapRegionSize | 4m / 8m / 16m / 32m в зависимости от heap |
| Metaspace | 128m / 256m / 512m в зависимости от heap |
| CodeCache | heap/16, в пределах 128-512m |
| SurvivorRatio | 32 (≤4 ядер) или 8 (>4 ядер) |
| Large Pages | включается только при наличии `SeLockMemoryPrivilege` |

### Вывод в stderr

```
[wrapper] System: 16 cores, 32.0GB total, 18.4GB free, large pages: yes
[wrapper] Heap: 9g | GC: parallel=14 concurrent=3 | Region: 16m
[wrapper] Flags: 28 injected, 3 removed
[wrapper] Started PID 12345
[wrapper] Process boosted
```

## Large Pages (опционально)

Для лучшей производительности включите large pages:

1. Запустите `secpol.msc`
2. Локальные политики → Назначение прав пользователя → Блокировка страниц в памяти
3. Добавьте своего пользователя, перезагрузитесь

Враппер определит это автоматически и добавит `-XX:+UseLargePages`.

## Сборка

```
go build -o wrapper.exe -ldflags="-s -w" .
```

## Требования

- Windows 10/11
- Права администратора (для `--install` / `--uninstall`)
- Установленный STALCRAFT

## Лицензия

MIT
