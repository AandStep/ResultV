export function profileDraft(profile) {
  return {
    host: profile.host || "",
    port: profile.port ? String(profile.port) : "",
    username: profile.username || "",
    password: profile.password || "",
    passwordEdited: false,
    sites: [...(profile.sites || [])],
    apps: [...(profile.apps || [])],
  };
}

const sameList = (a, b) => a.length === b.length && a.every((value, i) => value === b[i]);

export function isDraftDirty(draft, saved) {
  return draft.host !== saved.host || draft.port !== saved.port ||
    draft.username !== saved.username || draft.password !== saved.password ||
    !sameList(draft.sites, saved.sites) || !sameList(draft.apps, saved.apps);
}

export function addSite(list, value) {
  const site = value.trim().toLowerCase();
  return !site || list.includes(site) ? list : [...list, site];
}

export function addApp(list, value, isWin) {
  let name = value.trim();
  if (!name) return list;
  if (isWin && !name.toLowerCase().endsWith(".exe")) name += ".exe";
  if (!isWin && !name.includes(".")) name += ".app";
  return list.some((app) => app.toLowerCase() === name.toLowerCase()) ? list : [...list, name];
}

export function buildUpdate(draft) {
  return {
    host: draft.host.trim(),
    port: Number(draft.port),
    username: draft.username,
    password: draft.passwordEdited ? draft.password : null,
    sites: draft.sites,
    apps: draft.apps,
  };
}
