import * as m from '@/paraglide/messages';
import s from './{{Name}}Page.module.css';

export default function {{Name}}Page() {
  return (
    <main class={s.page}>
      <h1 class="text-2xl font-bold">{m.{{titleKey}}()}</h1>
    </main>
  );
}
