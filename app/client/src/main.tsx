import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import Markdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import "./site.css";

const dateFormat = new Intl.DateTimeFormat("en-US", { dateStyle: "long", timeZone: "UTC" });
const formatDate = (date: string) => dateFormat.format(new Date(date + "T00:00:00Z"));

type WritingSummary = {
  slug: string;
  title: string;
  date: string;
  summary?: string;
};

type WritingResult =
  | { status: "loading" | "missing" | "failed" }
  | { status: "list"; items: WritingSummary[] }
  | { status: "detail"; item: WritingSummary & { body: string } };

function isSummary(value: unknown): value is WritingSummary {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return typeof item.slug === "string" && typeof item.title === "string" &&
    typeof item.date === "string" && /^\d{4}-\d{2}-\d{2}$/.test(item.date) &&
    Number.isFinite(Date.parse(item.date)) &&
    (item.summary === undefined || typeof item.summary === "string");
}

function Writing({ slug }: { slug?: string }) {
  const [result, setResult] = useState<WritingResult>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    const url = slug ? "/api/v1/writing/" + encodeURIComponent(slug) : "/api/v1/writing";
    (async () => {
      try {
        const response = await fetch(url, { signal: controller.signal });
        if (slug && response.status === 404) {
          setResult({ status: "missing" });
          return;
        }
        if (!response.ok) throw new Error("Writing request failed");
        const data: unknown = await response.json();
        if (controller.signal.aborted) return;
        if (slug) {
          if (!isSummary(data) || data.slug !== slug || !("body" in data) || typeof data.body !== "string") {
            throw new Error("Invalid writing detail");
          }
          document.title = data.title + " · Bill I Am";
          setResult({ status: "detail", item: { ...data, body: data.body } });
        } else {
          if (!Array.isArray(data) || !data.every(isSummary)) throw new Error("Invalid writing list");
          setResult({ status: "list", items: data });
        }
      } catch {
        if (!controller.signal.aborted) setResult({ status: "failed" });
      }
    })();
    return () => controller.abort();
  }, [slug, attempt]);

  const { status } = result;
  return <>
    {slug && <p><a href="/writing">← Writing</a></p>}
    <h1>{status === "detail" ? result.item.title : "Writing"}</h1>
    {status === "loading" && <p role="status">Loading writing…</p>}
    {status === "missing" && <p role="status">This piece could not be found.</p>}
    {status === "failed" && <p role="status">Writing could not be loaded. <button onClick={() => {
      setResult({ status: "loading" });
      setAttempt(attempt + 1);
    }}>Retry</button></p>}
    {status === "detail" && <article>
      <time dateTime={result.item.date}>{formatDate(result.item.date)}</time>
      <Markdown skipHtml remarkPlugins={[remarkBreaks]}>{result.item.body}</Markdown>
    </article>}
    {status === "list" && (result.items.length ? <ul className="writing-list">
      {result.items.map((item) => <li key={item.slug}>
        <a href={"/writing/" + encodeURIComponent(item.slug)}>{item.title}</a>
        <time dateTime={item.date}>{formatDate(item.date)}</time>
        {item.summary && <p>{item.summary}</p>}
      </li>)}
    </ul> : <p>No writing has been published yet.</p>)}
  </>;
}

function App() {
  const path = window.location.pathname.replace(/\/$/, "") || "/";
  const detail = path.match(/^\/writing\/([a-z0-9]+(?:-[a-z0-9]+)*)$/);
  return <>
    <header>
      <a href="/">Bill I Am</a>
      <nav aria-label="Main navigation"><a href="/writing">Writing</a></nav>
    </header>
    <main>
      {path === "/" ? <>
        <h1>Bill I Am</h1>
        <p>Poems and other writing by Billy Burdon.</p>
        <p><a href="/writing">Read the writing</a></p>
      </> : path === "/writing" || detail ? <Writing slug={detail?.[1]} /> : <>
        <h1>Page not found</h1>
        <p><a href="/">Return home</a></p>
      </>}
    </main>
  </>;
}

const root = document.getElementById("root");
if (root) createRoot(root).render(<App />);
