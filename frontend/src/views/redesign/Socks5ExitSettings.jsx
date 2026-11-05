import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import { Button, Field, Textarea, Tumbler } from "../../components/kit";
import wailsAPI from "../../utils/wailsAPI";

function settingsForm(profile) {
  return {
    host: profile.host || "",
    port: profile.port || 1080,
    username: profile.username || "",
    password: "",
    hasPassword: Boolean(profile.hasPassword),
    clearPassword: false,
    domains: (profile.domains || []).join("\n"),
  };
}

export default function Socks5ExitSettings() {
  const { t } = useTranslation();
  const [form, setForm] = useState(null);
  const [status, setStatus] = useState(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const statusRevision = useRef(0);

  useEffect(() => {
    let mounted = true;
    let receivedStatus = false;
    const unsubscribe = EventsOn("status:update", (next) => {
      statusRevision.current++;
      receivedStatus = true;
      if (mounted) setStatus(next);
    });
    wailsAPI.getStatus().then((next) => {
      if (mounted && !receivedStatus) setStatus(next);
    }).catch((err) => {
      if (mounted) setMessage(String(err?.message || err));
    });
    wailsAPI.getSocks5ExitSettings().then((profile) => {
      if (mounted) setForm(settingsForm(profile));
    }).catch((err) => {
      if (mounted) setMessage(String(err?.message || err));
    });
    return () => {
      mounted = false;
      if (typeof unsubscribe === "function") unsubscribe();
    };
  }, []);

  const exit = status?.socks5Exit;
  const connected = status?.isConnected || status?.isEstablishing;
  const editable = Boolean(status && !connected && !exit?.enabled && !busy);
  const change = (key) => (event) => setForm({ ...form, [key]: event.target.value });

  const save = async (event) => {
    event.preventDefault();
    if (!editable) return;
    setBusy(true);
    setMessage("");
    try {
      await wailsAPI.saveSocks5ExitSettings({
        host: form.host,
        port: Number(form.port),
        username: form.username,
        password: form.password || (form.clearPassword ? "" : null),
        domains: form.domains.split(/\r?\n/),
      });
      setForm(settingsForm(await wailsAPI.getSocks5ExitSettings()));
      setMessage(t("socks5Exit.saved"));
    } catch (err) {
      setMessage(String(err?.message || err));
    } finally {
      setBusy(false);
    }
  };

  const toggle = async (enabled) => {
    const revision = statusRevision.current;
    setBusy(true);
    setMessage("");
    try {
      const next = await wailsAPI.setSocks5ExitEnabled(enabled);
      if (statusRevision.current === revision) {
        setStatus((prev) => ({ ...prev, socks5Exit: next }));
      }
    } catch (err) {
      setMessage(String(err?.message || err));
      const refreshRevision = statusRevision.current;
      const current = await wailsAPI.getStatus().catch(() => null);
      if (current && statusRevision.current === refreshRevision) setStatus(current);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Tumbler
        aria-label={t("socks5Exit.title")}
        checked={Boolean(exit?.enabled)}
        disabled={busy || (!exit?.available && !exit?.enabled)}
        onChange={toggle}
      />
      {connected && !exit?.available && (
        <p className="rv-settings-page__hint">{t("socks5Exit.requires")}</p>
      )}
      {form && (
        <form className="rv-settings-page__custom" onSubmit={save}>
          <Field label={t("socks5Exit.host")} value={form.host} onChange={change("host")} disabled={!editable} required />
          <Field label={t("socks5Exit.port")} type="number" min={1} max={65535} value={form.port} onChange={change("port")} disabled={!editable} required />
          <Field label={t("socks5Exit.username")} value={form.username} onChange={change("username")} disabled={!editable} autoComplete="off" />
          <Field label={t("socks5Exit.password")} type="password" value={form.password} onChange={change("password")} disabled={!editable} autoComplete="new-password" />
          {form.hasPassword && !form.clearPassword && (
            <Button type="button" disabled={!editable} onClick={() => setForm({ ...form, username: "", password: "", clearPassword: true })}>
              {t("socks5Exit.clearPassword")}
            </Button>
          )}
          <label className="rv-settings-page__custom">
            <span className="rv-settings-page__label">{t("socks5Exit.domains")}</span>
            <Textarea value={form.domains} onChange={change("domains")} disabled={!editable} required />
          </label>
          <p className="rv-settings-page__hint">{t("socks5Exit.authHint")}</p>
          <Button type="submit" variant="green" disabled={!editable}>{t("socks5Exit.save")}</Button>
        </form>
      )}
      {message && <p className="rv-settings-page__hint" role="status">{message}</p>}
    </>
  );
}
