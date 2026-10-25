// FileDetail is the shared destination for unowned search hits and local node
// provenance. It lives inside Explore's layout route beside NodeDetail, so the
// navigator and Explore mode chrome remain standing while /api/prose resolves
// and reads the cited Markdown file. The hash is part of the contract rather
// than decoration: only #L<positive> addresses a source line, and an invalid
// locator never starts a fetch.

import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import type { BaselineDocView, NodeDriftView, ProseFile } from "../api";
import { prosePath } from "../api";
import Chip from "../components/Chip";
import CopyCmd from "../components/CopyCmd";
import { DriftCandidateDetails } from "../components/DriftFinding";
import Prose from "../components/Prose";
import { titleOf, useDocumentTitle, useFetch } from "../hooks";
import { PersonName } from "../people";
import "../components/instrument.css";
import "./drift.css";

const FILE_ROUTE_PREFIX = "/file/";

export default function FileDetail() {
  const location = useLocation();
  // Read the encoded pathname rather than React Router's decoded splat.
  // useParams decodes percent escapes after the router has already decoded
  // them once, which turns a literal filename substring such as "%2F" into
  // a path separator. Splitting the raw route first and decoding each segment
  // exactly once preserves both literal percent escapes and real slashes.
  const path = filePath(location.pathname);
  const visiblePath = path ?? location.pathname.slice(FILE_ROUTE_PREFIX.length);
  const line = sourceLine(location.hash);
  const request =
    path !== null && path !== "" && line !== null ? prosePath(path, line) : null;
  const { data: file, error, loading } = useFetch<ProseFile>(request);
  const locatorRef = useRef<HTMLDivElement>(null);
  // The tab names the file the way the address line does, basename and line
  // ("app.md:12"): the directory is what the reader already knows, the file
  // is what tells two tabs apart. An address that never fetches gets the
  // invalid-location heading instead of a file it will not show.
  useDocumentTitle(
    titleOf(
      line === null || path === null || path === ""
        ? "Invalid file location"
        : `${path.slice(path.lastIndexOf("/") + 1)}:${line}`,
    ),
  );

  // The locator does not exist until the asynchronous read has rendered. An
  // effect therefore performs the one scroll after that commit rather than
  // racing the fetch or trying to instrument Markdown source positions.
  useEffect(() => {
    if (file !== null) {
      locatorRef.current?.scrollIntoView?.();
    }
  }, [file]);

  let content: ReactNode;
  if (line === null || path === null || path === "") {
    content = <InvalidLocation path={visiblePath} hash={location.hash} />;
  } else if (error !== null) {
    content = <LoadError path={path} line={line} error={error} />;
  } else if (loading || file === null) {
    content = (
      <>
        <FileAddress path={path} line={line} />
        <p className="muted">Loading file...</p>
      </>
    );
  } else {
    content = (
      <>
        <FileAddress path={file.path} line={file.line} />
        <div
          ref={locatorRef}
          id={`L${file.line}`}
          className="file-source-locator"
        >
          <span className="file-source-line" aria-hidden="true">
            L{file.line}
          </span>
          <code>{file.line_text}</code>
        </div>
        {file.baseline !== undefined && (
          <BaselineDetail key={file.baseline.path} baseline={file.baseline} drift={file.drift_detail} />
        )}
        <Prose markdown={file.content} />
      </>
    );
  }

  return <div className="explore-detail">{content}</div>;
}

function BaselineDetail({ baseline, drift }: { baseline: BaselineDocView; drift?: NodeDriftView }) {
  const [expanded, setExpanded] = useState<string[]>([]);
  const candidates = drift?.candidates ?? [];
  const uncovered = drift?.uncovered ?? [];
  const markers = [...new Set(candidates.map((row) => row.marker))];
  const stamp = baseline.confirmed;
  const approve = `tiller approve design -- '${baseline.path.replace(/'/g, "'\\''")}'`;
  return (
    <section className="drift-section" aria-label="As-is baseline">
      <h2 className="eyebrow">As-is baseline</h2>
      <p>
        <Chip status="draft">{stamp === undefined ? "Unconfirmed" : "Confirmed"}</Chip>
        {stamp !== undefined && <> by <PersonName id={stamp.by} /> at commit <span className="mono">{stamp.commit.value}</span>
          {stamp.commit.observed_at !== undefined && <> on <time dateTime={stamp.commit.observed_at}>{stamp.commit.observed_at}</time></>}
        </>}
      </p>
      {stamp !== undefined && <p className="muted">Confirmation records reviewed working-tree code, not a claim that those bytes were committed or remain current.</p>}
      <h3 className="eyebrow">Describes</h3>
      <ul className="rule-list">
        {baseline.describes.map((path) => <li key={path} className="mono">{path}</li>)}
      </ul>
      {(baseline.derived_from?.length ?? 0) > 0 && (
        <>
          <h3 className="eyebrow">Derived from</h3>
          <ul className="rule-list">
            {baseline.derived_from?.map((ref) => (
              <li key={`${ref.path}-${ref.value}`}>
                <span className="mono">{ref.path}</span>{" · "}<span className="mono">{ref.value}</span>
                {ref.observed_at !== undefined && <> observed <time dateTime={ref.observed_at}>{ref.observed_at}</time></>}
              </li>
            ))}
          </ul>
          <p className="muted">Recorded source blobs remain recoverable with <code>git show &lt;blob-sha&gt;</code>, even when the source file was retired.</p>
        </>
      )}
      {markers.map((marker) => {
        const open = expanded.includes(marker);
        return (
          <div key={marker}>
            <button type="button" className="instr-drift-chip" aria-expanded={open}
              onClick={() => setExpanded((current) => open ? current.filter((value) => value !== marker) : [...current, marker])}>
              <Chip status="draft">{marker}</Chip>
              <span className="chevron-glyph" aria-hidden="true">{open ? "▾" : "▸"}</span>
            </button>
            {open && <div className="instr-drift-panel">
              {candidates.filter((row) => row.marker === marker).map((row, i) => (
                <div key={i} className="drift-row"><DriftCandidateDetails candidate={row} /></div>
              ))}
            </div>}
          </div>
        );
      })}
      {uncovered.map((row, i) => (
        <p key={`${row.marker}-${i}`} className="muted instr-drift-uncovered">
          <span className="instr-drift-say">{row.marker}</span> not checked ({row.reason})
          {row.detail && <>: {row.detail}</>}
        </p>
      ))}
      {stamp === undefined && (
        <div className="instr-drift">
          <span>Human review required: reconcile this document against its sources and current code, then confirm it.</span>
          <CopyCmd cmd={approve} />
        </div>
      )}
      <Link to="/drift" className="muted instr-drift-more">project-wide drift report</Link>
    </section>
  );
}

// The browser route stores the repo path as independently encoded segments.
// Decode after splitting so an encoded slash can never change the segment
// boundaries, and treat malformed percent escapes as an invalid location.
function filePath(pathname: string): string | null {
  if (!pathname.startsWith(FILE_ROUTE_PREFIX)) {
    return null;
  }
  const rawPath = pathname.slice(FILE_ROUTE_PREFIX.length);
  try {
    return rawPath
      .split("/")
      .map((segment) => decodeURIComponent(segment))
      .join("/");
  } catch {
    return null;
  }
}

// Accept the canonical fragment and nothing nearby: no missing hash, zero,
// leading zero, sign, suffix, or number JavaScript cannot represent exactly.
function sourceLine(hash: string): number | null {
  const match = /^#L([1-9]\d*)$/.exec(hash);
  if (match === null) {
    return null;
  }
  const line = Number(match[1]);
  return Number.isSafeInteger(line) ? line : null;
}

function FileAddress({ path, line }: { path: string; line: number }) {
  return <p className="mono file-address">{path}:{line}</p>;
}

function InvalidLocation({ path, hash }: { path: string; hash: string }) {
  return (
    <section>
      <h2>Invalid file location</h2>
      {(path !== "" || hash !== "") && (
        <p className="mono file-address">{path}{hash}</p>
      )}
      <p>File links need an exact positive source line such as #L7.</p>
    </section>
  );
}

function LoadError({ path, line, error }: { path: string; line: number; error: string }) {
  if (error.includes(" failed (404")) {
    return (
      <section>
        <h2>No such file</h2>
        <FileAddress path={path} line={line} />
        <p>This search destination no longer exists in the working tree.</p>
      </section>
    );
  }
  return (
    <section>
      <h2>Could not load file</h2>
      <FileAddress path={path} line={line} />
      <p className="error">{error}</p>
    </section>
  );
}
