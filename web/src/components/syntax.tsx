import { CopyIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { clipboard } from '@/utils/clipboard';
import { cn } from '@/utils/cn';

interface SyntaxProps {
  code: string;
  className?: string;
  language?: 'typescript' | 'bash' | 'json' | 'markdown';
  wrapLines?: boolean;
  copyable?: boolean;
}

export default function Syntax({
  code,
  className,
  language = 'typescript',
  wrapLines = false,
  copyable = true,
}: SyntaxProps) {
  const [Highlighter, setHighlighter] = useState<any>(null);
  const [style, setStyle] = useState<any>(null);

  useEffect(() => {
    let mounted = true;
    Promise.all([
      import('react-syntax-highlighter'),
      import('react-syntax-highlighter/dist/esm/languages/hljs/bash'),
      import('react-syntax-highlighter/dist/esm/languages/hljs/json'),
      import('react-syntax-highlighter/dist/esm/languages/hljs/markdown'),
      import('react-syntax-highlighter/dist/esm/languages/hljs/typescript'),
      import('react-syntax-highlighter/dist/esm/styles/hljs/vs2015'),
    ]).then(([sh, bash, json, markdown, ts, docco]) => {
      if (!mounted) return;
      sh.Light.registerLanguage('typescript', ts.default);
      sh.Light.registerLanguage('json', json.default);
      sh.Light.registerLanguage('bash', bash.default);
      sh.Light.registerLanguage('markdown', markdown.default);
      setStyle(docco.default);
      setHighlighter(() => sh.Light);
    }).catch(console.error);
    return () => {
      mounted = false;
    };
  }, []);

  return (
    <div className={cn('group relative rounded-lg', className)}>
      {copyable && (
        <button
          className="row absolute top-1 right-1 items-center gap-2 rounded bg-card p-2 opacity-0 transition-opacity group-hover:opacity-100 z-10"
          onClick={() => {
            clipboard(code, null);
          }}
          type="button"
        >
          <span>Copy</span>
          <CopyIcon size={12} />
        </button>
      )}
      {Highlighter && style ? (
        <Highlighter
          customStyle={{
            borderRadius: 'var(--radius)',
            padding: '1rem',
            paddingTop: '0.5rem',
            paddingBottom: '0.5rem',
            fontSize: 14,
            lineHeight: 1.3,
          }}
          language={language}
          style={style}
          wrapLongLines={wrapLines}
        >
          {code}
        </Highlighter>
      ) : (
        <pre className="overflow-x-auto rounded-[var(--radius)] bg-def-200/50 p-4 text-[14px] leading-[1.3] font-mono">
          <code>{code}</code>
        </pre>
      )}
    </div>
  );
}
