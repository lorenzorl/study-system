/**
 * Fixture deck: the four slide shapes the deck parser has to produce.
 *
 * Static HTML on purpose, for this slice only — it is the input the markdown
 * parser will emit later, so the surface being rehearsed here is "given HTML,
 * mount a deck", not "given a note, build HTML". There is no inline `<script>`
 * and no inline event handler anywhere in this markup, and there never can be:
 * the deck viewer hands this string straight to `v-html` (see `DeckViewer.vue`),
 * so anything executable in it would run with the plugin's privileges.
 *
 * Layout classes (`deck-columns`, `deck-bento`, `deck-bento__cell`) are defined by
 * the plugin, not by the deck: PRESENTATION-UI.md Q4 keeps the grid in the plugin
 * so decks stay declarative and cannot fork the visual language.
 */
export const FIXTURE_DECK_HTML = `
<section>
  <h1>Atención</h1>
  <p>Cómo el sistema cognitivo elige qué procesar</p>
  <p><small>4 diapositivas · generado desde 6 notas</small></p>
</section>

<section>
  <h2>Atención selectiva</h2>
  <div class="deck-columns">
    <div>
      <h3>Qué filtra</h3>
      <ul>
        <li>Un estímulo entre varios simultáneos</li>
        <li>Lo relevante para la meta actual</li>
        <li>Lo que rompe la expectativa</li>
      </ul>
    </div>
    <div>
      <h3>Qué cuesta</h3>
      <p>
        Filtrar es trabajo: lo descartado sigue compitiendo por la entrada. La
        carga se nota como latencia, no como error.
      </p>
    </div>
  </div>
</section>

<section>
  <h2>Cuatro ideas</h2>
  <div class="deck-bento">
    <div class="deck-bento__cell">
      <h3>Filtro</h3>
      <p>Selección temprana del estímulo relevante.</p>
    </div>
    <div class="deck-bento__cell">
      <h3>Capacidad</h3>
      <p>El recurso es limitado y se agota con la tarea.</p>
    </div>
    <div class="deck-bento__cell">
      <h3>Control</h3>
      <p>La intención sostiene el foco frente a la distracción.</p>
    </div>
    <div class="deck-bento__cell">
      <h3>Coste</h3>
      <p>Cambiar de foco cuesta más que mantenerlo.</p>
    </div>
  </div>
</section>

<section>
  <h1>Fin</h1>
  <p>Volvé a las notas para repasar las tarjetas.</p>
  <p><small>Fuentes: Atención - Kahneman · Memoria de trabajo - Baddeley</small></p>
</section>
`.trim()
