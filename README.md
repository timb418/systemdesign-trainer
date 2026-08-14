# System Design Trainer

Локальный тренажёр подготовки к system design interview: банк задач по типам архитектур, интервьюер в чате (неполное ТЗ, уточняющие вопросы, deep dive), встроенная схема и разбор по рубрике. Стек — Go + HTMX, всё на машине пользователя, ключ OpenRouter свой.

Продуктовый контекст, функциональные и нефункциональные требования: [docs/SPEC.md](docs/SPEC.md).

## Запуск

Нужен Go 1.25+.

```bash
go run ./cmd/trainer
```

Откройте [http://127.0.0.1:8080](http://127.0.0.1:8080). Адрес только `127.0.0.1` (можно задать `SDT_ADDR=127.0.0.1:8080`).

В [Настройках](http://127.0.0.1:8080/settings) укажите ключ OpenRouter. Ключ пишется в `$XDG_CONFIG_HOME/systemdesign-trainer/openrouter.key` (права 0600). Сессии — SQLite в `$XDG_DATA_HOME/systemdesign-trainer/sessions.db`.

Доска: по умолчанию iframe `embed.diagrams.net`. Чтобы самохостить редактор:

```bash
./scripts/fetch-drawio.sh
```

XML схемы при этом всё равно хранится у нас, не на diagrams.net.

Сейчас в банке одна задача (`url-shortener-v1`) — чтобы прогнать интерфейс. Остальные карточки добавим отдельно.
