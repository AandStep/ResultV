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

import resultvBadge from "../../assets/resultv-buy-badge.svg";
import { Button, Icon } from "../../components/kit";
import PageHeader from "./PageHeader";
import "./BuyPage.css";

export default function BuyPage({
  subscriptionURL,
  copyMessage,
  connecting,
  onConnectTelegram,
  onOpenBot,
  onCopySubscription,
  sidebar,
  contentRef,
  text,
}) {
  return (
    <div className="rv-buy-page">
      {sidebar}

      <div ref={contentRef} className="rv-buy-page__content rv-scroll">
        <PageHeader title={text.title} subtitle={text.subtitle} />

        <div className="rv-buy-page__list">
          <section className="rv-buy-page__card">
            <div className="rv-buy-page__head">
              <img src={resultvBadge} alt="" className="rv-buy-page__logo" width="96" height="96" />
              <div className="rv-buy-page__text">
                <h2 className="rv-buy-page__title">{text.offerTitle}</h2>
                <p className="rv-buy-page__desc">{text.offerDescription}</p>
              </div>
            </div>

            <div className="rv-buy-page__actions">
              <Button
                variant="green"
                className="rv-buy-page__icon-btn"
                title={text.connect}
                aria-label={text.connect}
                disabled={connecting}
                icon={<Icon name="tg" color="currentColor" />}
                onClick={onConnectTelegram}
              />
              <Button
                className="rv-buy-page__purchase"
                icon={<Icon name="tg" color="var(--rv-white-a50)" />}
                onClick={onOpenBot}
              >
                {text.purchase}
              </Button>
            </div>
            {connecting && <p className="rv-buy-page__desc" role="status">{text.connecting}</p>}
            {text.connectionError && (
              <p className="rv-buy-page__error" role="alert">{text.connectionError}</p>
            )}
          </section>

          {subscriptionURL && (
            <section className="rv-buy-page__card">
              <div className="rv-buy-page__text">
                <h2 className="rv-buy-page__title">{text.subscriptionTitle}</h2>
                <p className="rv-buy-page__desc">{text.subscriptionDescription}</p>
              </div>
              <Button
                className="rv-buy-page__subscription-url"
                title={text.copySubscription}
                aria-label={text.copySubscription}
                icon={<Icon name="copy" color="currentColor" />}
                onClick={onCopySubscription}
              >
                <span aria-live="polite">{copyMessage || subscriptionURL}</span>
              </Button>
            </section>
          )}
        </div>
      </div>
    </div>
  );
}
