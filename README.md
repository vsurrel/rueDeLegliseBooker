# Déploiement rapide

## Démarrage via systemd (service utilisateur)

Voici un exemple d’unité `systemd` pour exécuter l’application située dans `/home/valentin/rueDeLegliseBooker` sous l’utilisateur `valentin`. Le service redémarre automatiquement en cas d’arrêt inattendu (délai de 10 s).

Fichier `~/.config/systemd/user/rueDeLegliseBooker.service` :

```ini
[Unit]
Description=Planning de reservation - rueDeLegliseBooker
After=network.target

[Service]
Type=simple
WorkingDirectory=/home/valentin/rueDeLegliseBooker
ExecStart=/home/valentin/rueDeLegliseBooker/rueDeLegliseBooker
Restart=always
RestartSec=10
Environment=PORT=64512

[Install]
WantedBy=default.target
```

### Activer le service au boot (même sans session graphique)

```bash
# Activer le lingering pour l'utilisateur valentin
loginctl enable-linger valentin

# Recharger les unités utilisateur et activer le service
sudo -u valentin systemctl --user daemon-reload
sudo -u valentin systemctl --user enable --now rueDeLegliseBooker.service
```

Le service démarrera désormais automatiquement après un redémarrage du serveur, même sans session utilisateur ouverte.

## Reverse proxy nginx

Ajoutez le bloc suivant dans votre configuration nginx pour exposer l’application (chemin `/paris`) vers le backend en écoute sur `http://localhost:64512` :

```nginx
location /paris/ {
    proxy_pass              http://localhost:64512/;
    proxy_set_header        Host                $host;
    proxy_set_header        X-Real-IP           $remote_addr;
    proxy_set_header        X-Forwarded-For     $proxy_add_x_forwarded_for;
    proxy_set_header        X-Forwarded-Proto   $scheme;
    proxy_set_header        X-Forwarded-Prefix  /paris;
    proxy_http_version      1.1;
    proxy_set_header        Connection          "";
    proxy_read_timeout      60s;
}
```

Rechargez ensuite nginx :

```bash
sudo nginx -t && sudo systemctl reload nginx
```
