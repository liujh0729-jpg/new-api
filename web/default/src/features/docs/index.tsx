/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import ReactMarkdown, { type Components } from 'react-markdown'
import rehypeRaw from 'rehype-raw'
import remarkGfm from 'remark-gfm'
import { PublicLayout } from '@/components/layout'
import userGuide from '../../../../../docs/aipdd-user-guide.zh_CN.md'

const documentComponents: Components = {
  h1: ({ node, ...props }) => (
    <h1
      {...props}
      className='scroll-m-20 text-3xl font-semibold tracking-tight sm:text-4xl'
    />
  ),
  h2: ({ node, ...props }) => (
    <h2
      {...props}
      className='border-border mt-12 scroll-m-20 border-b pb-3 text-2xl font-semibold tracking-tight'
    />
  ),
  h3: ({ node, ...props }) => (
    <h3
      {...props}
      className='mt-9 scroll-m-20 text-xl font-semibold tracking-tight'
    />
  ),
  h4: ({ node, ...props }) => (
    <h4
      {...props}
      className='mt-7 scroll-m-20 text-lg font-semibold tracking-tight'
    />
  ),
  p: ({ node, ...props }) => (
    <p {...props} className='text-foreground/90 mt-4 leading-7' />
  ),
  ul: ({ node, ...props }) => (
    <ul {...props} className='my-4 ml-6 list-disc [&>li]:mt-2' />
  ),
  ol: ({ node, ...props }) => (
    <ol {...props} className='my-4 ml-6 list-decimal [&>li]:mt-2' />
  ),
  a: ({ node, ...props }) => (
    <a
      {...props}
      className='text-primary font-medium underline underline-offset-4'
      target='_blank'
      rel='noopener noreferrer'
    />
  ),
  blockquote: ({ node, ...props }) => (
    <blockquote
      {...props}
      className='border-primary/70 bg-muted/40 my-5 rounded-r-lg border-l-4 px-5 py-1 italic'
    />
  ),
  pre: ({ node, ...props }) => (
    <pre
      {...props}
      className='bg-muted/70 my-5 overflow-x-auto rounded-xl border p-4 text-sm leading-6 [&>code]:bg-transparent [&>code]:p-0'
    />
  ),
  code: ({ node, ...props }) => (
    <code
      {...props}
      className='bg-muted rounded px-1.5 py-0.5 font-mono text-[0.9em]'
    />
  ),
  table: ({ node, ...props }) => (
    <div className='my-6 w-full overflow-x-auto rounded-xl border'>
      <table {...props} className='w-full border-collapse text-sm' />
    </div>
  ),
  thead: ({ node, ...props }) => <thead {...props} className='bg-muted/60' />,
  th: ({ node, ...props }) => (
    <th
      {...props}
      className='border-b px-4 py-3 text-left font-semibold whitespace-nowrap'
    />
  ),
  td: ({ node, ...props }) => (
    <td {...props} className='border-t px-4 py-3 align-top leading-6' />
  ),
  hr: ({ node, ...props }) => <hr {...props} className='border-border my-10' />,
}

export function Docs() {
  return (
    <PublicLayout>
      <article className='mx-auto max-w-5xl py-6 pb-20 sm:py-10 sm:pb-24'>
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          rehypePlugins={[rehypeRaw]}
          components={documentComponents}
        >
          {userGuide}
        </ReactMarkdown>
      </article>
    </PublicLayout>
  )
}
