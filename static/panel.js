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
