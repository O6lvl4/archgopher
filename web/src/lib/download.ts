export function download(name: string, text: string, type = "application/yaml"): void {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

/** Shows a document in a new tab; the browser keeps it until the tab closes. */
export function openInTab(text: string, type: string): void {
  const url = URL.createObjectURL(new Blob([text], { type }));
  if (!window.open(url, "_blank")) download("architecture.svg", text, type);
}

export function slug(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "architecture";
}
