# Reverse proxy с TLS для панели (опционально)

Панель по умолчанию слушает HTTP на `0.0.0.0:2053`. Для продакшена удобно
повесить её за nginx с сертификатом и Basic-allowlist:

```nginx
server {
    listen 443 ssl;
    server_name panel.example.com;

    ssl_certificate     /etc/letsencrypt/live/panel.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/panel.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:2053;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

После этого в `/etc/systemd/system/h2-ui.service` смените:

```
Environment=H2UI_ADDR=127.0.0.1:2053
```

и `systemctl daemon-reload && systemctl restart h2-ui` — панель будет
недоступна снаружи напрямую, только через nginx.
