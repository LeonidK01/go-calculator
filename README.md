# Calculator: Python → Go

Самостоятельный Go-проект с двумя программами: `calculator_server` и `generator`.
Исходные Python-файлы и библиотеки в родительской папке не изменены.
В runtime нет сторонних Go-зависимостей и Python; C и Rust вызываются через настоящий cgo/ABI.

## Быстрый запуск на Windows, macOS и Linux

Нужен работающий Docker Engine / Docker Desktop. Выполнить из этой папки:

На Windows есть помощник, который находит Docker Desktop даже без `docker` в PATH:

```powershell
.\run.ps1 start
.\run.ps1 load
.\run.ps1 stop
```

Для проверки сборки и тестов: `.\run.ps1 test`. Универсальные команды Docker:

```sh
docker compose up --build -d calculator
curl -X POST "http://localhost:8080/calc?num=42"
curl "http://localhost:8080/metrics"
docker compose --profile load run --rm generator
docker compose logs calculator
docker compose down
```

В PowerShell при конфликте псевдонима `curl` использовать `curl.exe`.
Генератор по умолчанию в Compose работает 60 секунд с 16 конкурентными работниками.
Его можно запустить с другими параметрами:

```sh
docker compose --profile load run --rm generator --url http://calculator:8080/calc --threads 32 --interval 0 --duration 10
```

`docker compose down` посылает SIGTERM. Сервер прекращает приём соединений,
ожидает активные запросы, выводит `[final] sum=... sub=...` и только затем выгружает `.so`.
Состояние хранится в памяти одного процесса и обнуляется при перезапуске, как в оригинале.

## Обычная Linux-сборка

Требования: Go 1.26+, GCC, Rust/Cargo, Bash, glibc и libdl.
Dockerfile фиксирует проверенную среду: Go 1.26.4, Rust 1.89, Debian Bookworm.

```sh
bash build.sh
./bin/calculator_server --host 0.0.0.0 --port 8080 --interval 5
# В другом терминале:
./bin/generator --url http://localhost:8080/calc -n 10 --interval 0.1 --timeout 5
```

Результат сборки: два бинарника и две `.so` в `bin/`.
По умолчанию сервер ищет библиотеки рядом со своим бинарником, независимо от текущего каталога.
Можно передать `--c-lib /path/libcalculator.so --rust-lib /path/libcalculator_rust.so`.
Сервер требует Linux/cgo. На других платформах сборка явно сообщает об этом при запуске,
без подмены нативных функций Go-реализацией. Генератор работает самостоятельно и с `CGO_ENABLED=0`.

## HTTP-контракт

| Запрос | Результат |
| --- | --- |
| `POST /calc?num=42` | `200`, тело ровно `ok`, оба вычисления уже завершены |
| `POST /calc` или пустой `num` | `400`, отсутствует параметр |
| `POST /calc?num=1.5` или повреждённая query string | `400`, неверное целое число |
| `GET /metrics` | `200`, Prometheus text format 0.0.4 |
| Неправильный метод известного маршрута | `405` с `Allow` |
| Неизвестный маршрут | `404` |

При повторении `num` используется первое значение. Поддерживаются отрицательные числа,
пробелы вокруг числа, знак `+` (в URL кодировать как `%2B`), разделители `1_000`
и десятичные Unicode-цифры. Большие целые приводятся к `int64` по модулю 2⁶⁴,
как при передаче Python `int` в `ctypes.c_int64`. Частый путь `int64` обходится без `big.Int`.
Переполнение итогов также определено по модулю 2⁶⁴.
Ограничение HTTP-заголовков — 16 KiB; это ограничивает и размер request target.

Параметры CLI сохранены: `--host`, `--port`, `--c-lib`, `--rust-lib`, `--interval`
у сервера; `--url`, `--threads` / `-n`, `--interval`, `--timeout` у генератора.
Времена принимаются в **секундах**, включая дробные значения. Добавлены:

- сервер: `--concurrency N`, по умолчанию `GOMAXPROCS`;
- генератор: `--duration N`, по умолчанию `0` — до SIGINT/SIGTERM.

Генератор выбирает число равномерно из `[-100, 100]`, переиспользует HTTP-соединения,
читает и закрывает ответы, считает `2xx` успешными, а другие статусы/таймауты — ошибками.
Редиректы не выполняются: они считаются ошибками, чтобы не скрывать неверный endpoint.
Ошибки агрегируются в финальном отчёте, без вывода строки на каждый сбой.
Отменённые при завершении незаконченные запросы не входят в `ok/errors`.
Финальная строка содержит `ok`, `errors`, время и средний RPS.

## Метрики

| Семейство | Смысл |
| --- | --- |
| `calculator_http_requests_total` | Все HTTP-запросы, дошедшие до handler, включая ошибки и `/metrics` |
| `calculator_http_rps{age_seconds="1"..."60"}` | Ровно 60 значений: количество запросов за каждую завершённую секунду |
| `calculator_rps_window_end_timestamp_seconds` | Исключительная верхняя граница окна, Unix time |
| `calculator_native_call_duration_seconds{library="c\|rust",quantile="0.95\|0.99"}` | p95/p99 нативных вызовов в секундах за 60 завершённых секунд |
| `calculator_native_call_duration_seconds_count{library="c\|rust"}` | Количество вызовов с запуска |
| `calculator_native_call_duration_seconds_sum{library="c\|rust"}` | Сумма измеренных длительностей с запуска, в секундах |

В обозначении `c\|rust` в таблице перечислены два возможных значения label: `c` и `rust`.

Если snapshot снят в `12:00:10.450`, `age_seconds="1"` соответствует
`[12:00:09, 12:00:10)`, а `"60"` — `[11:59:10, 11:59:11)`.
Текущая незавершённая секунда не выдаётся как полный RPS. Пропуски и секунды до старта — нули.
На первом scrape и после минуты без вычислений quantile равен `NaN`, а не ложному нулю.
Время запроса фиксируется при входе в handler, время наблюдения пары вызовов — после их завершения.

Замер включает cgo-переход и возврат, а также возможное вытеснение потока ОС;
ожидание очереди, обновление итогов и HTTP-ответ в него не входят.
Это elapsed time вызова, а не чистое CPU-time тела функции.

Percentile — верхняя граница bucket, содержащего nearest-rank p95/p99.
Гистограмма имеет 32 подразделения на степень двойки: ошибка по значению времени
менее 3,125%; это не обещание ошибки по рангу. Длительности до 63 ns представлены точно.
Память ограничена: около 2 MiB на 61 секундный слот для обеих функций независимо от RPS.
Счётчики `count/sum` накопительные, quantile скользящие, как у Prometheus summary.
Нельзя усреднять p95 разных процессов для получения общего p95.

Labels `age_seconds` ограничены 60 значениями, поэтому каждый scrape не создаёт
новые time series с уникальным timestamp. Для обычного графика RPS удобнее:

```promql
rate(calculator_http_requests_total[1m])
```

Пример конфигурации scrape: [configs/prometheus.yml](configs/prometheus.yml).

## Почему быстрее

Исходный Python удерживает один замок во время C `add` и Rust `sub`.
В этой задаче функции чистые, а сложение/вычитание ассоциативны по модулю 2⁶⁴:
можно вычислить `add(0, num)` и `sub(0, num)` независимо для разных запросов,
а затем под коротким mutex прибавить оба приращения к итогам.
Ответ отправляется после commit: очереди фоновых вычислений за ответом `ok` нет.

Внутри одного запроса C и Rust вызываются последовательно. Создание двух горутин
не оправдано: исходный Rust-цикл не имеет наблюдаемых эффектов и удаляется оптимизатором;
добавлять `black_box` ради искусственно большого Rust p99 означало бы менять задачу.
Нативная C-нагрузка с `volatile` сохранена. Исправлено исходное UB signed overflow в C,
а Rust использует `wrapping_sub`, одинаково работающее в debug/release.

Количество одновременно выполняемых нативных расчётов ограничено. Это защищает от
бесконтрольного роста числа cgo-потоков; ожидающий запрос можно отменить через context.
Оптимизация применима именно к приложенным чистым арифметическим функциям.
Подмена `.so` на функции с побочными эффектами или иной зависимостью от аккумулятора
потребует пересмотра алгоритма.

Замеры и ограничения: [docs/VALIDATION.md](docs/VALIDATION.md).
Архитектура и решения: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
Использованные навыки: [docs/SKILLS.md](docs/SKILLS.md).

## Проверки

Все проверки настоящих библиотек в изолированной Linux-среде:

```sh
docker build --target test -t go-calculator-test .
docker run --rm -v "$PWD:/app:ro" -w /app golangci/golangci-lint:v2.11.3 golangci-lint run
```

Для Linux с установленными инструментами:

```sh
bash build.sh
export CALCULATOR_NATIVE_DIR="$PWD/bin"
cargo test --manifest-path native/rust/Cargo.toml
go vet ./...
go test -race -coverprofile=coverage.out ./...
go test -tags=integration -race ./...
go test -run '^$' -fuzz=FuzzQuery -fuzztime=3s -parallel=4 ./internal/httpapi
go test -tags=integration -run '^$' -bench . -benchmem -cpu=1,4 ./tests ./internal/native ./internal/metrics
```

Обычный `go test ./...` на Windows проверяет ненативную логику.
Нативные unit-тесты без `CALCULATOR_NATIVE_DIR` явно пропускаются; интеграционные
тесты требуют этот параметр и собранные бинарники. Они проверяют настоящие `.so`,
параллельные HTTP-запросы, генератор и SIGTERM отдельному процессу сервера.

CI определён в `.github/workflows/ci.yml` для размещения этой папки как корня репозитория.
