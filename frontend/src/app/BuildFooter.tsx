interface BuildFooterProps {
  version?: string;
  commit?: string;
}

export function BuildFooter({
  version = import.meta.env.VITE_APP_VERSION,
  commit = import.meta.env.VITE_APP_COMMIT,
}: BuildFooterProps) {
  const label = version?.trim() || "dev";
  const sha = commit?.trim() ?? "";
  const validCommit = /^[0-9a-f]{40}$/i.test(sha);
  const shortSha = sha.slice(0, 7);

  return (
    <footer className="app-build-footer">
      Julius {label}
      {validCommit && (
        <>
          {" · "}
          <a
            href={`https://github.com/greg0x46/julius/commit/${sha}`}
            aria-label={`Ver commit ${shortSha} no GitHub`}
          >
            {shortSha}
          </a>
        </>
      )}
    </footer>
  );
}
