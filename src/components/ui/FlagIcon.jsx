import React, { useState } from "react";
import { Globe, Server } from "lucide-react";

export const FlagIcon = ({ code, className = "" }) => {
  const [imgError, setImgError] = useState(false);

  if (!code || code === "unknown" || code === "🌐")
    return <Globe className="w-6 h-6 text-zinc-500 shrink-0" />;
  if (code === "local" || code === "🏠")
    return <Server className="w-6 h-6 text-zinc-500 shrink-0" />;

  let isoCode = null;

  if (/^[a-zA-Z]{2}$/.test(code)) {
    isoCode = code.toLowerCase();
  } else {
    const clean = code.replace(/[\uFE0F]/g, "").trim();
    if (clean.length > 0) {
      const cp1 = clean.codePointAt(0);
      if (cp1 >= 0x1f1e6 && cp1 <= 0x1f1ff) {
        const cp2 = clean.codePointAt(2);
        if (cp2 >= 0x1f1e6 && cp2 <= 0x1f1ff) {
          isoCode =
            String.fromCharCode(cp1 - 0x1f1e6 + 97) +
            String.fromCharCode(cp2 - 0x1f1e6 + 97);
        }
      }
    }
  }

  if (isoCode && !imgError) {
    return (
      <img
        src={`https://cdnjs.cloudflare.com/ajax/libs/flag-icons/7.2.3/flags/4x3/${isoCode}.svg`}
        alt={isoCode.toUpperCase()}
        className={`block shrink-0 object-contain ${className}`}
        style={{ width: "24px", height: "18px", borderRadius: "2px" }}
        onError={() => setImgError(true)}
      />
    );
  }

  return (
    <span className="text-sm font-bold uppercase tracking-wider text-zinc-300 shrink-0 font-sans">
      {isoCode || code}
    </span>
  );
};
