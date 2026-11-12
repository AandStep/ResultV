import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Eye, EyeOff } from "lucide-react";
import { Button, Input, TagField, Tumbler } from "../../components/kit";
import { FlagIcon } from "../../components/ui/FlagIcon";
import { useConfigContext } from "../../context/ConfigContext";
import { useConnectionContext } from "../../context/ConnectionContext";
import wailsAPI from "../../utils/wailsAPI";
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import { addApp, addSite, buildUpdate, isDraftDirty, profileDraft } from "./socks5Draft";

// A write can finish after the user navigates away from the settings page.
let pendingWrite = Promise.resolve();

const capitalize = (text) => text.charAt(0).toUpperCase() + text.slice(1);

export function socks5ReasonText(t, status) {
  return t(`socks5Exit.reasons.${status.reason}`, {
    code: status.code,
    defaultValue: t("socks5Exit.reasons.probe_failed"),
  });
}

export default function Socks5ExitSettings() {
  const { t } = useTranslation();
  const { platform } = useConfigContext();
  const { isConnected } = useConnectionContext();
  const isWin = platform === "win32" || platform === "windows" || platform === "win64";
  const [settings, setSettings] = useState(null);
  const [draft, setDraft] = useState(null);
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(false);
  const [status, setStatus] = useState(null);
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const saved = settings && profileDraft(settings);
  const dirty = draft && saved && isDraftDirty(draft, saved);

  useEffect(() => {
    let mounted = true;
    pendingWrite.then(() => wailsAPI.getSocks5ExitSettings()).then((profile) => {
      if (!mounted) return;
      setSettings(profile);
      setDraft(profileDraft(profile));
    }).catch((err) => {
      if (mounted) setError(String(err?.message || err));
    });
    wailsAPI.getSocks5ExitStatus().then((next) => {
      if (mounted && next?.checked) setStatus(next);
    }).catch(() => {});
    const off = EventsOn("socks5:status", (next) => {
      if (!mounted) return;
      setChecking(false);
      setStatus(next?.checked ? next : null);
    });
    return () => {
      mounted = false;
      off?.();
    };
  }, []);

  useEffect(() => {
    if (!isConnected) setStatus(null);
  }, [isConnected]);

  const writeSettings = (write) => {
    setBusy(true);
    setError("");
    pendingWrite = pendingWrite.then(async () => {
      try {
        await write();
        const next = await wailsAPI.getSocks5ExitSettings();
        setSettings(next);
        setDraft(profileDraft(next));
      } catch (err) {
        setError(String(err?.message || err));
      } finally {
        setBusy(false);
      }
    });
  };

  const save = () => {
    if (!dirty || busy || !settings.enabled) return;
    const update = buildUpdate(draft);
    const cleared = !update.host && !draft.port && !draft.username && !draft.password;
    if (!cleared && (!update.host || !Number.isInteger(update.port) || update.port < 1 || update.port > 65535)) {
      setError(t("socks5Exit.invalidAddress"));
      return;
    }
    const addressChanged = saved.host !== update.host || settings.port !== update.port;
    if (addressChanged && saved.password && draft.username && !draft.passwordEdited) {
      setError(t("socks5Exit.passwordForNewAddress"));
      return;
    }
    if (Boolean(draft.username) !== Boolean(draft.password)) {
      setError(t("socks5Exit.credentialsPair"));
      return;
    }
    writeSettings(() => wailsAPI.saveSocks5ExitSettings(update));
  };

  const toggle = (enabled) => {
    if (!enabled) setShowPassword(false);
    writeSettings(() => wailsAPI.setSocks5ExitEnabled(enabled));
  };

  const check = () => {
    setChecking(true);
    wailsAPI.checkSocks5Exit().then((next) => {
      setStatus(next?.checked ? next : null);
    }).catch((err) => {
      setError(String(err?.message || err));
    }).finally(() => setChecking(false));
  };

  const editList = (key, next) => {
    setError("");
    setDraft((prev) => ({ ...prev, [key]: next }));
  };

  const field = (key, extra = {}) => (
    <Input
      aria-label={t(`socks5Exit.${key}`)}
      placeholder={t(`socks5Exit.${key}`)}
      value={draft[key]}
      onChange={(event) => {
        setError("");
        setDraft({
          ...draft, [key]: event.target.value, passwordEdited: draft.passwordEdited || key === "password",
        });
      }}
      disabled={busy}
      {...extra}
    />
  );

  const statusLine = () => {
    if (!settings?.host) return null;
    if (!isConnected) return t("socks5Exit.statusAfterConnect");
    if (checking) return t("socks5Exit.checking");
    if (!status) return t("socks5Exit.statusUnchecked");
    if (!status.ok) return capitalize(socks5ReasonText(t, status));
    return (
      <>
        {t("socks5Exit.statusOk", { ip: status.ip })}
        {status.country && (
          <>
            {" · "}
            <FlagIcon code={status.country} className="rv-settings-page__socks5-flag" />
            {" "}{status.country}
          </>
        )}
        {" · "}{t(status.udp ? "socks5Exit.udpYes" : "socks5Exit.udpNo")}
      </>
    );
  };

  return (
    <section
      className="rv-settings-page__card rv-border rv-border--static rv-settings-page__row rv-settings-page__socks5"
      aria-label={t("socks5Exit.title")}
    >
      <div className="rv-settings-page__socks5-header">
        <div className="rv-settings-page__row-text">
          <p className="rv-settings-page__row-title">{t("socks5Exit.title")}</p>
          <p className="rv-settings-page__row-desc">
            {t(settings?.enabled && !settings.host ? "socks5Exit.enterAddress" : "socks5Exit.desc")}
          </p>
        </div>
        <Tumbler
          aria-label={t("socks5Exit.title")}
          checked={Boolean(settings?.enabled)}
          disabled={!settings || busy}
          onChange={toggle}
        />
      </div>
      {settings?.enabled && draft && (
        <div className="rv-settings-page__socks5-fields">
          <div className="rv-settings-page__socks5-pair" data-address>
            {field("host", { autoComplete: "off" })}
            {field("port", { type: "number", min: 1, max: 65535 })}
          </div>
          <div className="rv-settings-page__socks5-pair">
            {field("username", { autoComplete: "off" })}
            <div className="rv-settings-page__socks5-password">
              {field("password", { type: showPassword ? "text" : "password", autoComplete: "new-password" })}
              <button
                type="button"
                className="rv-settings-page__socks5-eye"
                aria-label={t(showPassword ? "socks5Exit.hidePassword" : "socks5Exit.showPassword")}
                disabled={busy}
                onClick={() => setShowPassword((shown) => !shown)}
              >
                {showPassword ? <EyeOff size={18} aria-hidden /> : <Eye size={18} aria-hidden />}
              </button>
            </div>
          </div>
          <div className="rv-settings-page__socks5-list">
            <p className="rv-settings-page__row-title">{t("socks5Exit.sites")}</p>
            <TagField
              aria-label={t("socks5Exit.sites")}
              values={draft.sites}
              onAdd={(value) => editList("sites", addSite(draft.sites, value))}
              onRemove={(value) => editList("sites", draft.sites.filter((site) => site !== value))}
              placeholder={t("socks5Exit.sitesPlaceholder")}
              removeLabel={t("socks5Exit.remove")}
              disabled={busy}
            />
            <p className="rv-settings-page__row-desc">{t("socks5Exit.sitesHint")}</p>
          </div>
          <div className="rv-settings-page__socks5-list">
            <p className="rv-settings-page__row-title">{t("socks5Exit.apps")}</p>
            <TagField
              aria-label={t("socks5Exit.apps")}
              values={draft.apps}
              onAdd={(value) => editList("apps", addApp(draft.apps, value, isWin))}
              onRemove={(value) => editList("apps", draft.apps.filter((app) => app !== value))}
              placeholder={t("socks5Exit.appsPlaceholder")}
              removeLabel={t("socks5Exit.remove")}
              disabled={busy}
            />
            <p className="rv-settings-page__row-desc">{t("socks5Exit.appsHint")}</p>
          </div>
          <div className="rv-settings-page__socks5-actions">
            <Button variant="green" disabled={!dirty || busy} onClick={save}>
              {t("common.save")}
            </Button>
            <Button disabled={!isConnected || checking || busy || !settings.host} onClick={check}>
              {t("socks5Exit.check")}
            </Button>
          </div>
          {statusLine() && (
            <p className="rv-settings-page__row-desc rv-settings-page__socks5-status" role="status">
              {statusLine()}
            </p>
          )}
        </div>
      )}
      {error && <p className="rv-settings-page__hint" role="alert">{error}</p>}
    </section>
  );
}
