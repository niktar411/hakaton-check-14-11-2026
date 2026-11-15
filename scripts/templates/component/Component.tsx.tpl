import { type JSX, splitProps } from 'solid-js';
import { cx } from '@/shared/lib/cx';
import s from './{{Name}}.module.css';

export type {{Name}}Props = JSX.HTMLAttributes<HTMLDivElement>;

export function {{Name}}(props: {{Name}}Props) {
  const [local, rest] = splitProps(props, ['class', 'children']);
  return (
    <div {...rest} class={cx(s.root, local.class)}>
      {local.children}
    </div>
  );
}
