# Диагностика

- **AirPlay не обнаруживается**: проверьте `yam ps`, состояние службы, mDNS/Avahi, наличие соседних устройств в сети и конфликты с существующим Shairport Sync. Имя приёмника: `Я.Станция`.
- **Нет звука**: проверьте доступ к ALSA, совпадение устройства и отсутствие конкурирующих процессов. Профиль текущей версии рассчитан на Plum.
- **Нет визуализатора**: проверьте поддержку `display.clock.segment.v1`, логи о получении PCM из FIFO и ошибки HTTP Runtime API YAM.
- **Нет кнопок/обратной синхронизации громкости**: это ограничения v1.1.5, а не ошибка установки.
- **`yam install` падает с `another yam transaction is running` или
  `Resource temporarily unavailable (os error 11)`**: Luna держит общую
  блокировку (shared flock) на `/tmp/yam/yam.lock`, а YAM для транзакции
  берёт исключительную, и они несовместимы. Проверить и обойти:

  ```sh
  flock -n /tmp/yam/yam.lock -c true || echo lock-is-held
  ```

  Пока блокировка держится, повторы не помогут. Остановите Luna, поставьте
  пакет, поднимите Luna обратно — плагин включится сам (хук ждёт Luna около
  20 минут) либо включите его в интерфейсе:

  ```sh
  kill $(ps | grep '[l]una --listen' | awk '{print $1}')   # лок отпускается
  yam --platform <platform> --profiles /data/yam/platforms \
      install -y airplay/airplay-luna
  setsid /data/yam/bin/luna --listen 127.0.0.1:8099 \
      --data-dir /data/yam/var/luna/data --yam-root /data/yam \
      --yam-run /tmp/yam >>/tmp/luna.log 2>&1 </dev/null &
  ```

  После перезапуска Luna блокировка снимается. Тот же лок мешает и
  `yam remove`.
- **Другой тип Станции**: на данный момент не поддерживается готовым профилем. См. `PORTABILITY.md`.

Для обращения об ошибке приложите модель, архитектуру, версию yamd, `yam ps` и обезличенные логи. Не публикуйте `YAM_API_TOKEN`, MAC-адреса, персональные данные и закрытые ключи.
