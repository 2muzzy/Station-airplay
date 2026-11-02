# Station AirPlay 2.0

AirPlay 2 для Яндекс Станций под управлением [Luna](https://github.com/nyakokitsu/luna).
Один плагин, общий звук с Алисой, голосовое управление и установка одной командой
из реестра YAM v2.

## Что нового в 2.0

- **Установка одной командой** из реестра YAM v2: `yam install airplay/airplay-luna`.
  Вместе с плагином ставится и Luna, если её ещё нет.
- **Автозапуск.** Пакетный хук после установки сам включает плагин в Luna —
  отдельно ходить в веб-интерфейс не нужно. При следующей загрузке станции
  плагин стартует вместе с Luna.
- **Голосовое управление включено по умолчанию** на всех профилях, включая
  Мини и Миди: «поставь на паузу», «следующий трек», «продолжи» и т.д.
  Алиса при этом остаётся рабочей.
- **Громкость принадлежит Luna.** Слайдер отправителя ходит через
  `/api/volume`; если Luna не отвечает, плагин пишет в ALSA сам.
- **D-Bus без внешних утилит.** Собственный клиент сам делает handshake на
  приватной шине, затем на системной; `dbus-send` больше не требуется.
- **Все актуальные платы**: Станция 2, Миди, Станция 3, Лайт, Лайт 2, Мини,
  Мини 2, Мини 3.

## Поддерживаемые модели

| Станция | YAM model | Имя в AirPlay |
|---|---|---|
| Станция 2 | `yandexstation_2` | Yandex Station 2 AirPlay |
| Станция Миди | `yandexmidi` | Я.Станция |
| Станция 3 | `yandexstation_3` | Yandex Station 3 AirPlay |
| Лайт | `yandexmicro` | Yandex Station Lite AirPlay |
| Лайт 2 | `plum`, `yandexplum` | Yandex Station Light 2 AirPlay |
| Мини | `yandexmini` | Yandex Station Mini AirPlay |
| Мини 2 | `yandexmini_2` | Yandex Mini 2 AirPlay |
| Мини 3 | `bergamot` | Yandex Mini 3 AirPlay |
| Мини 3 Про | `yandexmini_3_pro` | Yandex Mini 3 Pro AirPlay |

На всех моделях звук общий с Алисой: приёмник подмешивается в тот же микшер,
поэтому голосовой ассистент продолжает работать во время воспроизведения.
Профиль выбирается автоматически по model ID, который отдаёт YAM.

## Установка

### Требования

- Станция с прошивкой, на которой работает YAM, и доступ по SSH.
- `yam` **0.2.5 или новее** на компьютере или на самой станции. Более старые
  клиенты не понимают поле `postinstall` в манифесте и откажутся ставить пакет.
- Свободные порты 5000 (или 7000 на Миди) и 8099 под Luna.

### Одна команда

Добавьте реестр (один раз) и поставьте плагин:

```sh
yam registry add official https://repo.yse.wtf/v2 \
  --key ed25519:5Rz8F47uftWVU5kLgWYhprkEZ4rpP2kecUge/a7KQas=

yam install airplay/airplay-luna
```

Всё. Пакет сам:
1. поставит Luna нужной версии, если её нет;
2. распакует плагин в `share/luna/plugins/airplay.airplay-luna`;
3. дождётся локального API Luna и включит плагин.

Через несколько секунд станция появится в меню AirPlay. Проверить, что приёмник
поднялся:

```sh
netstat -ltn | grep -E ':(5000|7000)'
```

Если Luna в момент установки не была запущена, хук всё равно сохранит выбор
плагина, и он стартует при следующем запуске Luna (или после перезагрузки
станции).

### Установка из файла

Скачанный `.yampkg` для своей модели ставится так же:

```sh
yam install ./station-airplay-2.0.0-yandexstation_2.yampkg
```

Артефакты для конкретных плат лежат в разделе Releases и в реестре
`airplay/airplay-luna`. Пакет для одной модели не встанет на другую — в
манифесте зафиксирован `target`.

## Настройка

После первого запуска Luna создаёт файл:

```text
/data/luna/plugins/plugin-state/airplay.airplay-luna/airplay-controls.json
```

Значения по умолчанию:

```json
{
  "voice": true,
  "native_volume": true,
  "ducking": false,
  "leds": true,
  "led_level": 200,
  "native_player": false,
  "alice_button": false
}
```

- `voice` — голосовые команды через runtime Luna (`ipc.speech` + `commands`).
- `native_volume` — громкость станции вместо громкости отправителя.
- `ducking` — приглушать AirPlay, когда говорит Алиса.
- `leds` — индикация на кольце/экране, где она поддерживается.
- `native_player` — отдавать воспроизведение встроенному плееру Luna.
- `alice_button` — пауза по кнопке Алисы на поддерживаемых платах.

После правки файла перезапустите плагин:

```sh
wget -q -O - --post-data= \
  http://127.0.0.1:8099/api/plugins/airplay.airplay-luna/service/disable
wget -q -O - --post-data= \
  http://127.0.0.1:8099/api/plugins/airplay.airplay-luna/service/enable
```

## Как это работает

```text
iPhone/Mac → AirPlay 2 → Shairport Sync → ALSA (общий микшер) → динамики
                                        └→ PCM FIFO → supervisor → Luna API
                                                                 ├→ голос
                                                                 ├→ громкость
                                                                 └→ LED/дисплей
```

- **Shairport Sync** принимает поток и декодирует его в PCM.
- **Supervisor** на Go держит дочерние процессы, синхронизирует громкость,
  отправляет голосовые команды и рисует индикацию через API Luna.
- **Avahi** публикует `_airplay._tcp`; **NQPTP** участвует в синхронизации
  AirPlay 2.
- Плагин не трогает GPIO и регистры LED напрямую — только документированный
  runtime API.

## Сборка из исходников

Нужен Go 1.23+. Полная сборка runtime и сторонних библиотек описана в
[source/BUILD.md](source/BUILD.md).

```sh
cd source
go test ./...
go vet ./...
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
  go build -buildvcs=false -trimpath -ldflags='-s -w' \
  -o ../bin/station-airplay .
```

Упаковка legacy `.yap` (для старого YAM):

```sh
./scripts/package.sh
```

Пакеты `.yampkg` для реестра YAM v2 собираются отдельным инструментом поверх
этих же бинарников и runtime-библиотек.

## Структура репозитория

```text
station-airplay/
├── source/            # Go-супервизор: профили, звук, controls, D-Bus
├── bin/, lib/         # ARMv7/AArch64 бинарники и библиотеки
├── config/            # конфигурация Avahi
├── scripts/           # сборка и упаковка
├── support/           # ручная подготовка и откат на устройстве
├── docs/              # модели, комнаты, диагностика, лицензии
├── tests/             # проверки миграции
├── plugin.toml        # манифест плагина
└── README.md
```

## Диагностика

**Станция не видна в AirPlay.** Проверьте, что приёмник слушает порт и что
mDNS проходит между сетями (VLAN/гостевой Wi-Fi часто режут multicast).

```sh
netstat -ltn | grep -E ':(5000|7000)'
tail -n 50 /data/luna/plugins/plugin-state/airplay.airplay-luna/runtime.log
```

**Нет звука.** Плагин играет в общий микшер станции; проверьте, что не запущена
вторая реализация AirPlay и что `amixer scontrols` видит нужный контрол.

**Не работают голосовые команды.** Luna должна отдавать в `/media` возможности
`speech` и `commands`. Если пакет ставился на старую Luna, обновите её и
перезапустите плагин.

**Плагин не стартует после установки.** Включите его вручную:

```sh
wget -q -O - --post-data= \
  http://127.0.0.1:8099/api/plugins/airplay.airplay-luna/service/enable
```

Больше сценариев — в [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md).

## Документация

- [docs/SHARED_MODELS.md](docs/SHARED_MODELS.md) — профили, общий звук, ограничения.
- [docs/SHARED_CONTROLS.md](docs/SHARED_CONTROLS.md) — голос, кнопки, громкость.
- [docs/ROOMS.md](docs/ROOMS.md) — имя комнаты в имени приёмника.
- [docs/ADD_STATION.md](docs/ADD_STATION.md) — добавление новой модели.
- [docs/AIRPLAY2.md](docs/AIRPLAY2.md) — детали AirPlay 2.
- [CHANGELOG.md](CHANGELOG.md) — история версий.

## Лицензии

Go-адаптер распространяется по GPL-3.0-or-later. Сторонние компоненты
(Shairport Sync, NQPTP, Avahi, D-Bus и другие) имеют собственные лицензии: см.
`LICENSES/` и [docs/NOTICE.md](docs/NOTICE.md). Происхождение бинарников
описано в [docs/THIRD_PARTY.md](docs/THIRD_PARTY.md).

## Отказ от ответственности

Проект не связан с Яндексом и Apple. Установка меняет поведение станции:
делайте резервную копию до изменений и используйте на свой риск.
