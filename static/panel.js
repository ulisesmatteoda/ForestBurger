// El único JS del panel: lo que htmx no puede hacer. Copiar al portapapeles solo se puede desde JS.
// Escucha los clics en todo el documento (y no en cada botón), así anda también en los modales que
// trae htmx después de cargar la página.

// COPIAR: un botón con data-copiar="texto" copia ese texto y avisa por 2 segundos.
document.addEventListener("click", async (evento) => {
  const boton = evento.target.closest("[data-copiar]");
  if (!boton) return;
  const original = boton.textContent;
  try {
    await navigator.clipboard.writeText(boton.dataset.copiar);
    boton.textContent = "¡COPIADO!";
  } catch {
    // Sin permiso (o sin HTTPS fuera de localhost): el texto igual se ve para copiarlo a mano.
    boton.textContent = "Copialo a mano";
  }
  setTimeout(() => { boton.textContent = original; }, 2000);
});

// ---------- Notificaciones push (Ajustes → Notificaciones) ----------
// Lo que htmx no puede hacer: pedir permiso al navegador, registrar el service worker (/sw.js) y
// suscribir este dispositivo. El servidor guarda la suscripción y avisa los pedidos nuevos.

// estadoPush muestra el estado de este dispositivo y el botón que corresponde.
async function estadoPush(caja) {
  const estado = caja.querySelector("[data-push-estado]");
  const mostrar = (...cuales) => caja.querySelectorAll("[data-push]").forEach((b) => { b.hidden = !cuales.includes(b.dataset.push); });
  if (!("serviceWorker" in navigator) || !("PushManager" in window) || !("Notification" in window)) {
    estado.textContent = "Este navegador no recibe notificaciones. En el iPhone, instalá la app (Compartir → Agregar a inicio) y abrila desde el ícono.";
    return mostrar();
  }
  if (Notification.permission === "denied") {
    estado.textContent = "Bloqueadas en el navegador: habilitá las notificaciones para este sitio en la configuración del celu o del navegador, y volvé a entrar.";
    return mostrar();
  }
  const registro = await navigator.serviceWorker.getRegistration("/admin/");
  const suscripcion = registro && await registro.pushManager.getSubscription();
  if (suscripcion) {
    estado.textContent = "Activadas en este dispositivo.";
    mostrar("probar", "desactivar");
  } else {
    estado.textContent = "Desactivadas en este dispositivo.";
    mostrar("activar");
  }
}

// La clave pública VAPID viene en base64url; el navegador la quiere en bytes.
function claveEnBytes(base64url) {
  const relleno = "=".repeat((4 - (base64url.length % 4)) % 4);
  const texto = atob((base64url + relleno).replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(texto, (c) => c.charCodeAt(0));
}

// activo espera a que el service worker recién registrado quede activo. No se usa
// navigator.serviceWorker.ready: espera un worker para la URL con la que se abrió el documento, y
// si el panel se abrió en /admin (Productos) y se llegó a Ajustes con hx-boost, esa URL no entra en
// el scope /admin/ y se queda esperando para siempre.
function activo(registro) {
  return new Promise((listo) => {
    const nuevo = registro.installing || registro.waiting;
    if (!nuevo) return listo(registro);
    nuevo.addEventListener("statechange", () => { if (nuevo.state === "activated") listo(registro); });
  });
}

async function mandarJSON(url, datos) {
  const r = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(datos) });
  if (!r.ok) throw new Error("el servidor respondió " + r.status);
}

// Cuando aparece la tarjeta (al cargar la página o al llegar a Ajustes con hx-boost).
document.addEventListener("htmx:load", (evento) => {
  const raiz = evento.detail.elt;
  const caja = raiz.matches && raiz.matches("[data-push-caja]") ? raiz : raiz.querySelector && raiz.querySelector("[data-push-caja]");
  if (caja) estadoPush(caja);
});

document.addEventListener("click", async (evento) => {
  const boton = evento.target.closest("[data-push]");
  if (!boton) return;
  const caja = boton.closest("[data-push-caja]");
  const estado = caja.querySelector("[data-push-estado]");
  boton.disabled = true;
  try {
    if (boton.dataset.push === "activar") {
      if (await Notification.requestPermission() !== "granted") return await estadoPush(caja);
      const registro = await activo(await navigator.serviceWorker.register("/sw.js", { scope: "/admin/" }));
      const suscripcion = await registro.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: claveEnBytes(caja.dataset.clave) });
      await mandarJSON("/admin/push/suscribir", suscripcion.toJSON());
    } else if (boton.dataset.push === "desactivar") {
      const registro = await navigator.serviceWorker.getRegistration("/admin/");
      const suscripcion = registro && await registro.pushManager.getSubscription();
      if (suscripcion) {
        await mandarJSON("/admin/push/desuscribir", { endpoint: suscripcion.endpoint });
        await suscripcion.unsubscribe();
      }
    } else if (boton.dataset.push === "probar") {
      const r = await fetch("/admin/push/prueba", { method: "POST" });
      estado.textContent = await r.text();
      return;
    }
    await estadoPush(caja);
  } catch (error) {
    estado.textContent = "No se pudo: " + error.message;
  } finally {
    boton.disabled = false;
  }
});
