const urls = import.meta.glob<string>("./icons/**/*.svg", { eager: true, query: "?url", import: "default" });

/**
 * A node's picture. `name` is "<provider>/<icon>" from the catalog; the label
 * next to it names the service, so the picture itself is decorative.
 */
export function Icon({ name, size, className }: { name: string | undefined; size: number; className?: string }) {
  const src = name ? urls[`./icons/${name}.svg`] : undefined;
  if (!src) return <span className={`icon-blank ${className ?? ""}`} style={{ width: size, height: size }} />;
  return <img className={className} src={src} alt="" width={size} height={size} draggable={false} />;
}
