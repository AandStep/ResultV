import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon, TagField } from "../../components/kit";
import wailsAPI from "../../utils/wailsAPI";
import "./SmartRulesPage.css";

const REFRESH_MS = 5000;

export default function AdaptiveVerdicts() {
  const { t } = useTranslation();
  const [data, setData] = useState({ items: [], hidden: 0 });

  useEffect(() => {
    let mounted = true;
    const load = () =>
      wailsAPI.getLearnedVerdicts().then((next) => {
        if (mounted && next) setData(next);
      }).catch(() => {});
    load();
    const timer = setInterval(load, REFRESH_MS);
    return () => {
      mounted = false;
      clearInterval(timer);
    };
  }, []);

  const run = (call) => {
    call().then((next) => next && setData(next)).catch(() => {});
  };

  const hosts = (data.items || []).map((item) => item.host);

  return (
    <section className="rv-smart-rules__card" aria-label={t("adaptiveVerdicts.title")}>
      <div className="rv-smart-rules__card-head">
        <div className="rv-smart-rules__card-text">
          <h2 className="rv-smart-rules__card-directions rv-smart-rules__card-title">
            {t("adaptiveVerdicts.title")}
          </h2>
          <p className="rv-smart-rules__card-subtitle">{t("adaptiveVerdicts.desc")}</p>
        </div>
        <button
          type="button"
          className="rv-smart-rules__clear"
          aria-label={t("adaptiveVerdicts.clear")}
          title={t("adaptiveVerdicts.clear")}
          onClick={() => run(() => wailsAPI.clearLearnedVerdicts())}
        >
          <Icon name="delete" size={24} color="currentColor" />
        </button>
      </div>
      <TagField
        className="rv-smart-rules__list"
        values={hosts}
        readOnly
        onRemove={(host) => run(() => wailsAPI.forgetLearnedVerdict(host))}
        placeholder={t("adaptiveVerdicts.empty")}
        removeLabel={t("smartRules.remove")}
      />
      {data.hidden > 0 && (
        <p className="rv-smart-rules__card-subtitle">
          {t("adaptiveVerdicts.hidden", { count: data.hidden })}
        </p>
      )}
    </section>
  );
}
