/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

/*
 * Мини-гайд о перестановке групп. Показывается один раз, при первом заходе на
 * страницу серверов (settings.reorderGuideSeen). Сцена — зацикленная
 * CSS-анимация: курсор подходит к верхней карточке, удерживает её, меняется на
 * «схватившую» руку, переносит карточку вниз и отпускает.
 */

import { mdiCursorDefault, mdiHandBackRight } from "@mdi/js";
import { Button, Dialog, Icon } from "../../components/kit";
import "./ReorderGuide.css";

function DemoCard({ index, title, my = false }) {
  return (
    <div className={`rv-reorder-guide__card rv-reorder-guide__card--${index}`}>
      <span className="rv-reorder-guide__logo" data-my={my || undefined}>
        {my ? <Icon name="dns" size={16} color="currentColor" /> : title.slice(0, 1)}
      </span>
      <span className="rv-reorder-guide__card-title">{title}</span>
      <Icon name="open" size={16} color="currentColor" className="rv-reorder-guide__chevron" />
    </div>
  );
}

export default function ReorderGuide({ open, text, onClose }) {
  if (!open) return null;

  return (
    <Dialog
      variant="success"
      icon="sort"
      title={text.title}
      subtitle={text.subtitle}
      onClose={onClose}
      className="rv-reorder-guide"
      actions={
        <Button variant="green" onClick={onClose}>
          {text.ok}
        </Button>
      }
    >
      <div className="rv-reorder-guide__stage" aria-hidden="true">
        <div className="rv-reorder-guide__stack">
          <DemoCard index={0} title={text.subA} />
          <DemoCard index={1} title={text.subB} />
          <DemoCard index={2} title={text.my} my />
        </div>

        <div className="rv-reorder-guide__cursor">
          <svg className="rv-reorder-guide__ring" viewBox="0 0 40 40">
            <circle cx="20" cy="20" r="16" />
          </svg>
          <svg className="rv-reorder-guide__pointer rv-reorder-guide__pointer--arrow" viewBox="0 0 24 24">
            <path d={mdiCursorDefault} />
          </svg>
          <svg className="rv-reorder-guide__pointer rv-reorder-guide__pointer--grab" viewBox="0 0 24 24">
            <path d={mdiHandBackRight} />
          </svg>
        </div>

        <span className="rv-reorder-guide__hint rv-reorder-guide__hint--hold">{text.hold}</span>
        <span className="rv-reorder-guide__hint rv-reorder-guide__hint--drag">{text.drag}</span>
      </div>

      <p className="rv-reorder-guide__text">{text.text}</p>
    </Dialog>
  );
}
