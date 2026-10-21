// The only module that may call fetch, and only for same-origin asset paths (spec §3).
export function instantiateStreaming(
  url: string,
  imports: WebAssembly.Imports,
): Promise<WebAssembly.WebAssemblyInstantiatedSource> {
  return WebAssembly.instantiateStreaming(fetch(url), imports);
}
