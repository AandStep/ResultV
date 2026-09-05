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
 * Страница «Купить сервера». Figma "ResultV" -> App Design, фрейм BuyServers
 * (6613:3136).
 *
 * Фрейм один, состояний у него нет. Раскладка простая: шапка страницы, под
 * ней карточка партнёра — знак 96 на подложке, название с описанием и ряд из
 * трёх кнопок: ТГ-бот, сайт и промокод. Карточек в макете одна, но рисуются
 * все, что придут: партнёр — это данные, а не вёрстка.
 */

import { Button, Icon } from "../../components/kit";
import PageHeader from "./PageHeader";
import "./BuyPage.css";

/* Подписи в написании макета. В приложении они идут через i18n. */
export const BUY_PAGE_TEXT = {
  /*
   * В макете (6613:3140) набрано «Купить сервер». В приложении страница
   * зовётся «Купить сервера» — так её назвали при переименовании из
   * «Сервера impVPN», см. GAPS.md.
   */
  title: "Купить сервера",
  subtitle:
    "Здесь вы можете приобрести лучшие подписки у наших партнеров с бонусами по нашему промокоду.",
  /* Подписей у кнопок-иконок в макете нет — это подсказки. */
  bot: "Открыть бота",
  site: "Перейти на сайт",
  copyPromo: "Скопировать промокод",
  copied: "Скопировано!",
};

/**
 * `partners` — карточки страницы сверху вниз. У каждой свои знак, название,
 * описание и промокод; кнопка бота или сайта появляется только если ссылка
 * на них есть.
 */
export default function BuyPage({
  title,
  subtitle,
  partners = [],
  /* id партнёра, чей промокод только что скопировали. */
  copiedId = "",
  onOpenBot,
  onOpenSite,
  onCopyPromo,
  sidebar,
  text = BUY_PAGE_TEXT,
  className = "",
  ...rest
}) {
  return (
    <div className={`rv-buy-page ${className}`} {...rest}>
      {sidebar}

      <div className="rv-buy-page__content rv-scroll">
        <PageHeader title={title ?? text.title} subtitle={subtitle ?? text.subtitle} />

        <div className="rv-buy-page__list">
          {partners.map((partner) => (
            <div key={partner.id} className="rv-buy-page__card">
              <div className="rv-buy-page__head">
                <div className="rv-buy-page__logo">
                  <img src={partner.logo} alt="" className="rv-buy-page__logo-img" />
                </div>
                <div className="rv-buy-page__text">
                  <h2 className="rv-buy-page__title">{partner.title}</h2>
                  <p className="rv-buy-page__desc">{partner.desc}</p>
                </div>
              </div>

              <div className="rv-buy-page__actions">
                {partner.botLink && (
                  <Button
                    variant="green"
                    className="rv-buy-page__icon-btn"
                    title={text.bot}
                    aria-label={text.bot}
                    icon={<Icon name="tg" color="currentColor" />}
                    onClick={() => onOpenBot?.(partner)}
                  />
                )}
                {partner.siteLink && (
                  <Button
                    variant="green"
                    className="rv-buy-page__icon-btn"
                    title={text.site}
                    aria-label={text.site}
                    icon={<Icon name="site" color="currentColor" />}
                    onClick={() => onOpenSite?.(partner)}
                  />
                )}
                <Button
                  className="rv-buy-page__promo"
                  title={text.copyPromo}
                  icon={<Icon name="copy" color="currentColor" />}
                  onClick={() => onCopyPromo?.(partner)}
                >
                  {copiedId === partner.id ? text.copied : partner.promo}
                </Button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
