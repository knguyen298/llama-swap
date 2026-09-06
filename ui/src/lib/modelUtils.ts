import type { Model } from "./types";

export interface GroupedModels {
  local: Model[];
  localMatching: Model[];
  peers: Model[];
}

/**
 * The UI reaches the model endpoints through their /api mirror
 * (/api/v1/..., /api/upstream/..., /api/comfyui/, /api/sdapi/...). The mirror
 * is guarded by the web UI's auth mode, while the public paths always require
 * an API key the browser does not have.
 */
export const MODEL_API_PREFIX = "/api";

export function modelServerPath(modelId: string): string {
  if (modelId === "comfyui_auto") return `${MODEL_API_PREFIX}/comfyui/`;
  return `${MODEL_API_PREFIX}/upstream/${encodeURIComponent(modelId)}/`;
}

export function matchesCapabilities(model: Model, required: string[], matchAny = false): boolean {
  if (!required.length) return true;
  if (!model.capabilities) return false;
  const caps = model.capabilities as Record<string, boolean>;
  if (matchAny) {
    return required.some((cap) => caps[cap] === true);
  }
  return required.every((cap) => caps[cap] === true);
}

export function groupModels(models: Model[], capabilities?: string[], matchAny = false): GroupedModels {
  const available = models.filter((m) => !m.unlisted);
  const local = available.filter((m) => !m.peerID);
  const peers = available.filter((m) => m.peerID);

  let localMatching: Model[] = [];
  let localRest: Model[] = [];

  if (capabilities && capabilities.length > 0) {
    for (const model of local) {
      if (matchesCapabilities(model, capabilities, matchAny)) {
        localMatching.push(model);
      } else {
        localRest.push(model);
      }
    }
  } else {
    localRest = local;
  }

  return { local: localRest, localMatching, peers };
}
