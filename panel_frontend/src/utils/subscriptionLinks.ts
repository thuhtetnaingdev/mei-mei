const browserOrigin = typeof window !== "undefined" ? window.location.origin : "http://localhost:8080";

function rewriteHttpUrl(value?: string | null): string {
  if (!value) {
    return "";
  }

  try {
    const parsed = new URL(value);
    return new URL(`${parsed.pathname}${parsed.search}${parsed.hash}`, browserOrigin).toString();
  } catch {
    if (value.startsWith("/")) {
      return new URL(value, browserOrigin).toString();
    }
    return value;
  }
}

function rewriteImportUri(value?: string | null): string {
  if (!value) {
    return "";
  }

  try {
    if (value.startsWith("sing-box://")) {
      const parsed = new URL(value);
      const remoteUrl = parsed.searchParams.get("url");
      if (remoteUrl) {
        parsed.searchParams.set("url", rewriteHttpUrl(remoteUrl));
      }
      return parsed.toString();
    }

    if (value.startsWith("hiddify://import/")) {
      const [schemePrefix, fragment = ""] = value.split("#", 2);
      const remoteUrl = schemePrefix.slice("hiddify://import/".length);
      return `hiddify://import/${rewriteHttpUrl(remoteUrl)}${fragment ? `#${fragment}` : ""}`;
    }
  } catch {
    return value;
  }

  return value;
}

export function localizeSubscriptionLinks<T extends {
  url?: string;
  remoteProfileUrl?: string;
  clashProfileUrl?: string;
  singboxProfileUrl?: string;
  singboxImportUrl?: string;
  hiddifyImportUrl?: string;
}>(
  value: T
): T {
  return {
    ...value,
    url: rewriteHttpUrl(value.url),
    remoteProfileUrl: rewriteHttpUrl(value.remoteProfileUrl),
    clashProfileUrl: rewriteHttpUrl(value.clashProfileUrl),
    singboxProfileUrl: rewriteHttpUrl(value.singboxProfileUrl),
    singboxImportUrl: rewriteImportUri(value.singboxImportUrl),
    hiddifyImportUrl: rewriteImportUri(value.hiddifyImportUrl)
  };
}
