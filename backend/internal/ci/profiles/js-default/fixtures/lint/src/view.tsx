type Props = { title: string; count?: number };

export function Badge({ title, count = 0 }: Props) {
  return <span title={title}>{count > 0 ? `${title}: ${count}` : title}</span>;
}
