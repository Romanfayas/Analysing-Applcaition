"use client";

import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const res = await fetch(`/api/auth/${mode}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(mode === "login" ? { email, password } : { email, password, name }),
      });
      const body = (await res.json().catch(() => ({}))) as { error?: string };
      if (!res.ok) {
        setError(body.error ?? "Sign-in failed");
        return;
      }
      const next = params.get("next");
      // Only same-site relative paths: never redirect to an attacker-supplied absolute URL.
      router.replace(next && next.startsWith("/") && !next.startsWith("//") ? next : "/");
      router.refresh();
    } catch {
      setError("Could not reach the server");
    } finally {
      setBusy(false);
    }
  }

  const input = "w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm focus:outline-none focus:border-blue-500";

  return (
    <div className="min-h-screen flex items-center justify-center p-6">
      <form onSubmit={submit} className="glass-card p-8 w-full max-w-sm space-y-4" aria-labelledby="auth-title">
        <div className="text-center mb-2">
          <h1 id="auth-title" className="text-2xl font-heading font-bold text-white">
            Halal<span className="text-emerald-400">Equity</span>
          </h1>
          <p className="text-sm text-slate-400 mt-1">{mode === "login" ? "Sign in to continue" : "Create your account"}</p>
        </div>

        {mode === "register" && (
          <label className="block text-xs text-slate-500 uppercase">
            Name
            <input className={input} value={name} onChange={(e) => setName(e.target.value)} required autoComplete="name" maxLength={120} />
          </label>
        )}
        <label className="block text-xs text-slate-500 uppercase">
          Email
          <input className={input} type="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoComplete="email" />
        </label>
        <label className="block text-xs text-slate-500 uppercase">
          Password
          <input
            className={input}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={mode === "register" ? 10 : undefined}
            autoComplete={mode === "login" ? "current-password" : "new-password"}
          />
          {mode === "register" && <span className="block normal-case text-[11px] text-slate-500 mt-1">At least 10 characters.</span>}
        </label>

        {error && (
          <p role="alert" className="text-sm text-rose-400 bg-rose-500/10 border border-rose-500/20 rounded-lg px-3 py-2">
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={busy}
          className="w-full py-2.5 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-60 text-white text-sm font-semibold transition-colors"
        >
          {busy ? "Please wait…" : mode === "login" ? "Sign in" : "Create account"}
        </button>

        <button
          type="button"
          onClick={() => {
            setMode(mode === "login" ? "register" : "login");
            setError(null);
          }}
          className="w-full text-xs text-slate-400 hover:text-slate-200"
        >
          {mode === "login" ? "No account? Create one" : "Have an account? Sign in"}
        </button>

        <p className="text-[11px] text-slate-600 text-center">Paper trading only. No live order execution.</p>
      </form>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
