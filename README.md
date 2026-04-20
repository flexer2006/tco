# tco

Локальный сервис для синхронизации сообщений Telegram в Obsidian-совместимый vault.

Пайплайн: `Telegram -> ONNX embeddings -> дедупликация/кластеризация -> Markdown + manifest`.

## Минимальные требования

- Telegram API credentials: `TELEGRAM_API_ID`, `TELEGRAM_API_HASH`
- Целевой чат: `TELEGRAM_CHAT_ID`
- Модель и артефакты в `./models`:
  - `all-MiniLM-L6-v2.onnx`
  - `tokenizer.json`
  - `libonnxruntime.so`

## Что за что отвечает

- `cmd/collector` — единственная точка входа сервиса.
- HTTP control plane (`127.0.0.1:8080` по умолчанию):
  - `/auth` — web-onboarding Telegram (логин/код/2FA).
  - `/pipeline/run` — запуск синхронизации.
  - `/pipeline/status` — статус последнего/текущего прогона.
  - `/healthz`, `/readyz` — health/readiness.
- `vault/` — результат синхронизации:
  - `vault/topics/<cluster>/index.md` и заметки `*.md`
  - `vault/_meta/manifest.json`
  - `vault/_meta/embeddings/*.json`
  - `vault/_meta/telegram.session.json`