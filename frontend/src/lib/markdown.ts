import MarkdownIt from "markdown-it";
import hljs from "highlight.js/lib/common";

// 单例 markdown 渲染器：代码块走 highlight.js 语法高亮，其余走标准 CommonMark。
// 输出的 HTML 由调用方以 innerHTML 注入；输入始终来自模型文本，markdown-it 默认转义 HTML，
// 未开启 html:true，因此不会执行内联 HTML/脚本。
const md: MarkdownIt = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: false,
  highlight(code, lang): string {
    const language = lang && hljs.getLanguage(lang) ? lang : "";
    let body: string;
    try {
      body = language
        ? hljs.highlight(code, { language, ignoreIllegals: true }).value
        : escapeHtml(code);
    } catch {
      body = escapeHtml(code);
    }
    const label = language || "text";
    // data-code 保存原始文本供“复制”按钮读取（base64 避免属性中的引号/换行问题）。
    const raw = encodeCodePayload(code);
    return (
      `<figure class="code-block" data-lang="${escapeAttr(label)}" data-code="${raw}">` +
      `<figcaption><span class="code-lang">${escapeHtml(label)}</span>` +
      `<button type="button" class="code-copy" data-role="copy">复制</button></figcaption>` +
      `<pre class="hljs"><code>${body}</code></pre>` +
      `</figure>`
    );
  },
});

// 链接统一在新窗口打开并加安全 rel。
const defaultLinkOpen =
  md.renderer.rules.link_open ??
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));
md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  token.attrSet("target", "_blank");
  token.attrSet("rel", "noreferrer noopener");
  return defaultLinkOpen(tokens, idx, options, env, self);
};

export function renderMarkdown(source: string): string {
  return md.render(source ?? "");
}

// 复制按钮的事件委托：挂在消息容器上，点到 data-role="copy" 就读同级 figure 的原始代码。
export function handleCodeCopyClick(event: MouseEvent): void {
  const target = event.target as HTMLElement | null;
  const button = target?.closest<HTMLElement>('[data-role="copy"]');
  if (!button) {
    return;
  }
  const figure = button.closest<HTMLElement>(".code-block");
  const payload = figure?.getAttribute("data-code");
  if (!payload) {
    return;
  }
  const text = decodeCodePayload(payload);
  void navigator.clipboard?.writeText(text).then(
    () => flashCopyLabel(button, "已复制"),
    () => flashCopyLabel(button, "复制失败"),
  );
}

function flashCopyLabel(button: HTMLElement, label: string): void {
  const original = button.dataset.original ?? button.textContent ?? "复制";
  button.dataset.original = original;
  button.textContent = label;
  button.classList.add("copied");
  window.setTimeout(() => {
    button.textContent = button.dataset.original ?? "复制";
    button.classList.remove("copied");
  }, 1400);
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function escapeAttr(value: string): string {
  return escapeHtml(value).replace(/"/g, "&quot;");
}

function encodeCodePayload(code: string): string {
  try {
    // 先 encodeURIComponent 处理多字节，再 btoa 得到 ASCII 安全串。
    return btoa(unescape(encodeURIComponent(code)));
  } catch {
    return "";
  }
}

function decodeCodePayload(payload: string): string {
  try {
    return decodeURIComponent(escape(atob(payload)));
  } catch {
    return "";
  }
}
