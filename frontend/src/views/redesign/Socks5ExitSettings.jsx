import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Eye, EyeOff } from "lucide-react";
import { Button, Input, Tumbler } from "../../components/kit";
import wailsAPI from "../../utils/wailsAPI";

// A write can finish after the user navigates away from the settings page.
let pendingWrite = Promise.resolve();

function profileDraft(profile) {
  return {
    host: profile.host || "",
    port: profile.port ? String(profile.port) : "",
    username: profile.username || "",
    password: profile.password || "",
    passwordEdited: false,
  };
}

export default function Socks5ExitSettings() {
  const { t } = useTranslation();
  const [settings, setSettings] = useState(null);
  const [draft, setDraft] = useState(null);
  const [busy, setBusy] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const saved = settings && profileDraft(settings);
  const dirty = draft && saved && (
    draft.host !== saved.host || draft.port !== saved.port ||
    draft.username !== saved.username || draft.password !== saved.password
  );

  useEffect(() => {
    let mounted = true;
    pendingWrite.then(() => wailsAPI.getSocks5ExitSettings()).then((profile) => {
      if (!mounted) return;
      setSettings(profile);
      setDraft(profileDraft(profile));
    }).catch((err) => {
      if (mounted) setError(String(err?.message || err));
    });
    return () => { mounted = false; };
  }, []);

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
    const host = draft.host.trim();
    const port = Number(draft.port);
    const cleared = !host && !draft.port && !draft.username && !draft.password;
    if (!cleared && (!host || !Number.isInteger(port) || port < 1 || port > 65535)) {
      setError(t("socks5Exit.invalidAddress"));
      return;
    }
    const addressChanged = saved.host !== host || settings.port !== port;
    if (addressChanged && saved.password && draft.username && !draft.passwordEdited) {
      setError(t("socks5Exit.passwordForNewAddress"));
      return;
    }
    if (Boolean(draft.username) !== Boolean(draft.password)) {
      setError(t("socks5Exit.credentialsPair"));
      return;
    }
    const password = draft.passwordEdited ? draft.password : null;
    writeSettings(() => wailsAPI.saveSocks5ExitSettings({ host, port, username: draft.username, password }));
  };

  const toggle = (enabled) => {
    if (!enabled) setShowPassword(false);
    writeSettings(() => wailsAPI.setSocks5ExitEnabled(enabled));
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
          <Button
            className="rv-settings-page__socks5-save"
            variant="green"
            disabled={!dirty || busy}
            onClick={save}
          >
            {t("common.save")}
          </Button>
        </div>
      )}
      {error && <p className="rv-settings-page__hint" role="alert">{error}</p>}
    </section>
  );
}
