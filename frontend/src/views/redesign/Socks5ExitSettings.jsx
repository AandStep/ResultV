import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input, Tumbler } from "../../components/kit";
import wailsAPI from "../../utils/wailsAPI";

export default function Socks5ExitSettings() {
  const { t } = useTranslation();
  const [settings, setSettings] = useState(null);
  const [draft, setDraft] = useState(null);
  const [switching, setSwitching] = useState(false);
  const [error, setError] = useState("");
  const committed = useRef(null);
  const writes = useRef(Promise.resolve());

  useEffect(() => {
    let mounted = true;
    wailsAPI.getSocks5ExitSettings().then((profile) => {
      if (!mounted) return;
      committed.current = profile;
      setSettings(profile);
      setDraft({
        host: profile.host || "",
        port: String(profile.port || 1080),
        username: profile.username || "",
        password: "",
      });
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
      const password = snapshot.password || null;
      if (previous.host === host && previous.port === port &&
          previous.username === snapshot.username && !password) return;
      if (previous.hasPassword && snapshot.username && !password &&
          (previous.host !== host || previous.port !== port)) {
        setError(t("socks5Exit.passwordForNewAddress"));
        return;
      }
      if (snapshot.username && !password && !previous.hasPassword) return;
      if (!snapshot.username && password) return;
      try {
        await wailsAPI.saveSocks5ExitSettings({ host, port, username: snapshot.username, password });
        const next = await wailsAPI.getSocks5ExitSettings();
        committed.current = next;
        setSettings(next);
        setDraft((current) => current === snapshot
          ? { host: next.host, port: String(next.port), username: next.username, password: "" }
          : current);
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
        setDraft({
          host: next.host || "", port: String(next.port || 1080),
          username: next.username || "", password: "",
        });
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
      onChange={(event) => setDraft({ ...draft, [key]: event.target.value })}
      onBlur={save}
      onKeyDown={(event) => { if (event.key === "Enter") event.currentTarget.blur(); }}
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
        <div className="rv-settings-page__socks5-fields">
          <div className="rv-settings-page__socks5-pair" data-address>
            {field("host", { autoComplete: "off" })}
            {field("port", { type: "number", min: 1, max: 65535 })}
          </div>
          <div className="rv-settings-page__socks5-pair">
            {field("username", { autoComplete: "off" })}
            {field("password", {
              type: "password",
              autoComplete: "new-password",
              placeholder: t(settings.hasPassword ? "socks5Exit.passwordSaved" : "socks5Exit.password"),
            })}
          </div>
        </div>
      )}
      {error && <p className="rv-settings-page__hint" role="alert">{error}</p>}
    </div>
  );
}
