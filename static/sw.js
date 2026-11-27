// Service worker del panel: SOLO para las notificaciones push (excepción a "sin JS propio",
// ver CLAUDE.md). No tiene manejador de fetch: no guarda nada en caché, la app no anda offline.

// Que tome el control enseguida, sin esperar a que se cierren las pestañas.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (evento) => evento.waitUntil(self.clients.claim()));

// Llega un pedido: la notificación con el ícono de la app. requireInteraction: no se va sola;
// renotify: suena y vibra aunque haya otra del mismo pedido.
self.addEventListener("push", (evento) => {
  const aviso = evento.data ? evento.data.json() : {};
  evento.waitUntil(self.registration.showNotification(aviso.titulo || "ForestBurger", {
    body: aviso.cuerpo || "",
    icon: "/static/icons/icon-192.png",
    badge: "/static/icons/icon-192.png",
    tag: aviso.tag || "forestburger",
    renotify: true,
    requireInteraction: true,
    vibrate: [300, 100, 300, 100, 600],
    data: { url: aviso.url || "/admin/pedidos" },
  }));
});

// Al tocarla: si el panel ya está abierto, lo trae al frente y lo lleva al pedido; si no, lo abre.
self.addEventListener("notificationclick", (evento) => {
  evento.notification.close();
  const url = new URL(evento.notification.data.url, self.location.origin).href;
  evento.waitUntil((async () => {
    const ventanas = await self.clients.matchAll({ type: "window" });
    for (const v of ventanas) {
      if (v.url.startsWith(self.location.origin + "/admin") && "navigate" in v) {
        await v.focus();
        return v.navigate(url);
      }
    }
    return self.clients.openWindow(url);
  })());
});
