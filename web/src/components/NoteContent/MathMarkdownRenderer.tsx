import "katex/dist/katex.min.css";
import rehypeKatex from "rehype-katex";
import { remarkCurrencySafeMath } from "@/utils/remark-plugins/remark-currency-safe-math";
import { NoteMarkdownRendererCore, type NoteMarkdownRendererProps } from "./NoteMarkdownRenderer";

const MathMarkdownRenderer = (props: NoteMarkdownRendererProps) => (
  <NoteMarkdownRendererCore
    {...props}
    mathRemarkPlugins={[remarkCurrencySafeMath]}
    mathRehypePlugins={[[rehypeKatex, { throwOnError: false, strict: false }]]}
  />
);

export default MathMarkdownRenderer;
