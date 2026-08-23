const CODEX_MUX_THREAD_API = "http://127.0.0.1:__CODEX_MUX_CONTROL_PORT__/v1";
const CODEX_MUX_THREAD_TOKEN = "__CODEX_MUX_CONTROL_TOKEN__";

async function codexMuxThreadRequest(path, options = {}) {
  const response = await fetch(`${CODEX_MUX_THREAD_API}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      "X-Codex-Mux-Token": CODEX_MUX_THREAD_TOKEN,
      ...options.headers,
    },
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `Request failed (${response.status})`);
  return body;
}

function CodexMuxThreadSubscription() {
  const route = $n(sr);
  const threadId =
    route.value.routeKind === "local-thread" ? route.value.conversationId : null;
  const [account, setAccount] = TE.useState(null);
  const [accounts, setAccounts] = TE.useState([]);
  const [busy, setBusy] = TE.useState(false);
  const [error, setError] = TE.useState("");

  TE.useEffect(() => {
    let active = true;
    if (!threadId) {
      setAccount(null);
      return () => {
        active = false;
      };
    }

    const refresh = async () => {
      try {
        const [threadResult, poolResult] = await Promise.all([
          codexMuxThreadRequest(
            `/thread-account?threadId=${encodeURIComponent(threadId)}`,
          ),
          codexMuxThreadRequest("/accounts"),
        ]);
        if (active) {
          setAccount(threadResult.account || null);
          setAccounts(poolResult.accounts || []);
          setError("");
        }
      } catch {
        if (active) setAccount(null);
      }
    };

    refresh();
    const events = new EventSource(
      `${CODEX_MUX_THREAD_API}/events?token=${encodeURIComponent(CODEX_MUX_THREAD_TOKEN)}`,
    );
    events.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (
          payload.type === "account-updated" ||
          ((payload.type === "thread-failed-over" ||
            payload.type === "thread-moved") &&
            payload.data?.threadId === threadId)
        ) {
          refresh();
        }
      } catch {}
    };
    const warmupTimer = setTimeout(refresh, 2_000);
    const timer = setInterval(refresh, 30_000);
    return () => {
      active = false;
      clearTimeout(warmupTimer);
      clearInterval(timer);
      events.close();
    };
  }, [threadId]);

  async function moveThread(event) {
    const accountId = event.currentTarget.value;
    if (!threadId || !accountId || accountId === account?.id || busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await codexMuxThreadRequest("/thread-account", {
        method: "PATCH",
        body: JSON.stringify({ threadId, accountId }),
      });
      setAccount(result.account || null);
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setBusy(false);
    }
  }

  if (!account) return null;
  const weekly = codexMuxThreadWeeklyWindow(account.rateLimits);
  const remaining = weekly == null ? null : Math.max(0, 100 - weekly.usedPercent);
  const depleted = remaining === 0;
  const AccountAvatar = globalThis.CodexMuxAccountAvatar;
  return (0, zE.jsx)(K.Section, {
    sectionKey: "codex-mux-subscription",
    title: "Subscription",
    children: (0, zE.jsxs)("div", {
      className: "flex flex-col gap-1 py-1 text-sm",
      children: [
        (0, zE.jsxs)("div", {
          className: "flex min-h-9 items-center justify-between gap-2",
          children: [
            AccountAvatar
              ? (0, zE.jsx)(AccountAvatar, {
                  imageUrl: account.profileImageUrl,
                  label: account.label,
                  className: "size-5 shrink-0",
                })
              : null,
            (0, zE.jsx)("select", {
              className:
                "min-w-0 flex-1 truncate rounded-md bg-transparent px-1 py-1 text-token-text-primary outline-none hover:bg-token-foreground/5 disabled:opacity-60",
              value: account.id,
              disabled: busy,
              title: "Move this chat to another subscription",
              "aria-label": "Move this chat to another subscription",
              onChange: moveThread,
              children: accounts
                .filter((candidate) => candidate.enabled && candidate.connected)
                .map((candidate) => {
                  const candidateWeekly = codexMuxThreadWeeklyWindow(
                    candidate.rateLimits,
                  );
                  const candidateRemaining =
                    candidateWeekly == null
                      ? null
                      : Math.max(0, 100 - candidateWeekly.usedPercent);
                  const identity = candidate.email
                    ? `${candidate.label} · ${candidate.email}`
                    : candidate.label;
                  const usage =
                    candidateRemaining == null
                      ? "usage unavailable"
                      : candidateRemaining === 0
                        ? "depleted"
                        : `${Math.round(candidateRemaining)}% left`;
                  return (0, zE.jsx)(
                    "option",
                    {
                      value: candidate.id,
                      disabled: candidateRemaining === 0,
                      children: `${identity} · ${usage}`,
                    },
                    candidate.id,
                  );
                }),
            }),
            (0, zE.jsx)("span", {
              className: "shrink-0 tabular-nums text-token-description-foreground",
              children: busy
                ? "Moving…"
                : remaining == null
                  ? "Usage unavailable"
                  : depleted
                    ? "Depleted"
                    : `${Math.round(remaining)}% remaining`,
            }),
          ],
        }),
        error
          ? (0, zE.jsx)("span", {
              className: "text-xs text-red-500",
              children: error,
            })
          : null,
      ],
    }),
  });
}

function codexMuxThreadWeeklyWindow(rateLimits) {
  const windows = [rateLimits?.primary, rateLimits?.secondary].filter(Boolean);
  windows.sort(
    (left, right) =>
      (left.windowDurationMins || 0) - (right.windowDurationMins || 0),
  );
  return windows.at(-1) || null;
}
