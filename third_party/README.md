# Сторонние компоненты

Этот каталог хранит исходные архивы, сведения о сборке, контрольные суммы и лицензионные материалы. Сами архивы не устанавливаются на станцию; пакет содержит собранные `bin/`, `lib/` и notices.

## Основные исходники

| Компонент | Версия | Источник |
|---|---|---|
| Shairport Sync | 5.6.2 | [Тег upstream](https://github.com/mikebrady/shairport-sync/tree/5.6.2) |
| NQPTP | 1.2.9 | [Тег upstream](https://github.com/mikebrady/nqptp/tree/1.2.9) |
| FFmpeg | 7.1.3 | [Исходный архив](https://ffmpeg.org/releases/ffmpeg-7.1.3.tar.xz) |
| Mbed TLS | 3.6.7 | [Релиз upstream](https://github.com/Mbed-TLS/mbedtls/releases/tag/mbedtls-3.6.7) |
| D-Bus | 1.16.2 | [Исходный архив](https://dbus.freedesktop.org/releases/dbus/dbus-1.16.2.tar.xz) |

Лицензии: Shairport Sync — GPL-3.0, NQPTP — GPL-2.0, выбранные компоненты FFmpeg — LGPL-2.1+, Mbed TLS — Apache-2.0 или GPL-2.0+, D-Bus — GPL-2.0+ или AFL-2.1. Сохраняйте конкретные notices из `licenses/` и `LICENSES/`; одинаковые тексты для разных архитектур хранятся в одном экземпляре.

[airplay2-build.json](airplay2-build.json) фиксирует dev-пакеты базовой сборки; [controls-runtime-build.json](controls-runtime-build.json) — входы control-enabled receiver. [SHA256SUMS.json](SHA256SUMS.json) фиксирует хеши бинарников, библиотек, notices и исходных архивов. Команды и патчи: [BUILD.md](../source/BUILD.md), `scripts/` и `support/`.

## Состав и ограничения

Старые Avahi 0.7 и часть ARMhf-библиотек переиспользуются из прежнего пакета. Их исходная воспроизводимость остаётся ограниченной: [THIRD_PARTY.md](../docs/THIRD_PARTY.md). Библиотеки прошивки — приватные входы связывания; их не следует распространять.

UPX 5.2.1 сжимает только статический Go-супервизор. UPX-runtime или декомпрессор на станцию не устанавливается.

`bin/dbus-daemon` и `bin/dbus-send`, добавленные для Мини в 1.4.3, собраны из D-Bus 1.16.2 через zig cc 0.16 для ARMhf: glibc-символы до 2.34, без RUNPATH. Они используют пакетные D-Bus/Expat/libc. `build-runtime.sh` пересобирает этот комплект с GCC и подготовленным sysroot.

## Приёмник с shared-управлением

ARMhf receiver использует Shairport Sync 5.6.2 с патчами HomePod, Mbed TLS, shared-controls, DACP/IPv4 и preplay event channel из `support/`. `scripts/build-controls-runtime.sh` включает D-Bus-интерфейс и собирает его отдельно от базового receiver.

Дополнительные ARMhf-библиотеки avahi-client, blkid, config, mount, pcre2-8, popt, resolv, selinux, sodium и uuid происходят из проверенного Debian buster-набора, совместимого с прежним Avahi. Их notices находятся в `LICENSES/controls-runtime`. `collect-runtime.py` переносит только недостающие публичные ELF-зависимости, а `matching-resolver.py` проверяет хеш libc перед добавлением соответствующего resolver.

Историческая сборка использовала кешированный Debian-образ. Полная чистая пересборка, закрепление исходников всех зависимостей и комплект для перелинковки остаются отдельными проверками перед бинарной публикацией. AArch64 shared-controls не квалифицированы. Не считайте наличие проверочных записей в JSON доказательством аппаратной приёмки нового runtime.
