export function download(name: string, text: string, type = "application/yaml"): void {
  openBlob(new Blob([text], { type }), name);
}

/** Shows a document in a new tab, or saves it under name when the tab is blocked. */
export function openBlob(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob);
  if (window.open(url, "_blank")) return;
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

/** Draws an SVG document into a PNG at scale pixels per unit, with the browser's own fonts. */
export function svgToPng(svg: string, scale = 2): Promise<Blob> {
  const width = Number(/\swidth="([\d.]+)"/.exec(svg)?.[1] ?? 0);
  const height = Number(/\sheight="([\d.]+)"/.exec(svg)?.[1] ?? 0);
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => {
      const canvas = document.createElement("canvas");
      canvas.width = Math.ceil((width || img.width) * scale);
      canvas.height = Math.ceil((height || img.height) * scale);
      const ctx = canvas.getContext("2d");
      if (!ctx) return reject(new Error("no canvas"));
      ctx.scale(scale, scale);
      ctx.drawImage(img, 0, 0);
      canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error("the PNG could not be made"))), "image/png");
    };
    img.onerror = () => reject(new Error("the SVG did not load"));
    img.src = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg);
  });
}

export function slug(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "architecture";
}
