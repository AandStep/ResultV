import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Eye, EyeOff } from "lucide-react";
import { Input, Tumbler } from "../../components/kit";
import wailsAPI from "../../utils/wailsAPI";

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
  const [switching, setSwitching] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const committed = useRef(null);
  const writes = useRef(Promise.resolve());

  useEffect(() => {
    let mounted = true;
    wailsAPI.getSocks5ExitSettings().then((profile) => {
      if (!mounted) return;
      committed.current = profile;
      setSettings(profile);
      setDraft(profileDraft(profile));
    }).catch((err) => {
      if (mounted) setError(String(err?.message || err));
    });
    return () => { mounted = false; };
  }, []);

  const save = () => {
    const snapshot = draft;
    if (!snapshot || !settings?.enabled) return;
    const host = snapshot.host.trim();
    const port = Number(snapshot.port);
    if (!host || !Number.isInteger(port) || port < 1 || port > 65535) {
      if (snapshot.host || settings.host) setError(t("socks5Exit.invalidAddress"));
      return;
    }

    writes.current = writes.current.then(async () => {
      const previous = committed.current;
      const addressChanged = previous.host !== host || previous.port !== port;
      if (!addressChanged && previous.username === snapshot.username &&
          previous.password === snapshot.password) {
        setError("");
        return;
      }
      if (addressChanged && previous.password && snapshot.username && !snapshot.passwordEdited) {
        setError(t("socks5Exit.passwordForNewAddress"));
        return;
      }
      if (Boolean(snapshot.username) !== Boolean(snapshot.password)) {
        setError(t("socks5Exit.credentialsPair"));
        return;
      }
      try {
        const password = snapshot.passwordEdited ? snapshot.password : null;
        await wailsAPI.saveSocks5ExitSettings({ host, port, username: snapshot.username, password });
        const next = await wailsAPI.getSocks5ExitSettings();
        committed.current = next;
        setSettings(next);
        setDraft((current) => current === snapshot ? profileDraft(next) : current);
        setError("");
      } catch (err) {
        setError(String(err?.message || err));
      }
    });
  };

  const toggle = async (enabled) => {
    setSwitching(true);
    setError("");
    try {
      await writes.current;
      await wailsAPI.setSocks5ExitEnabled(enabled);
      const next = await wailsAPI.getSocks5ExitSettings();
      committed.current = next;
      setSettings(next);
      if (!enabled) {
        setDraft(profileDraft(next));
        setShowPassword(false);
      }
    } catch (err) {
      setError(String(err?.message || err));
    } finally {
      setSwitching(false);
    }
  };

  const field = (key, extra = {}) => (
    <Input
      aria-label={t(`socks5Exit.${key}`)}
      placeholder={t(`socks5Exit.${key}`)}
      value={draft[key]}
      onChange={(event) => setDraft({
        ...draft, [key]: event.target.value, passwordEdited: draft.passwordEdited || key === "password",
      })}
      onKeyDown={(event) => { if (event.key === "Enter") save(); }}
      disabled={switching}
      {...extra}
    />
  );

  return (
    <div className="rv-settings-page__row rv-settings-page__socks5">
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
          disabled={!settings || switching}
          onChange={toggle}
        />
      </div>
      {settings?.enabled && draft && (
        <div
          className="rv-settings-page__socks5-fields"
          onBlur={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget)) save();
          }}
        >
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
                disabled={switching}
                onClick={() => setShowPassword((shown) => !shown)}
              >
                {showPassword ? <EyeOff size={18} aria-hidden /> : <Eye size={18} aria-hidden />}
              </button>
            </div>
          </div>
        </div>
      )}
      {error && <p className="rv-settings-page__hint" role="alert">{error}</p>}
    </div>
  );
}
