import { console } from "@ndn/util";
import LRUMap from "mnemonist/lru-map.js";

import { env } from "./env.js";

class RangeResponse {
  /**
   * @param {number} firstSegment
   * @param {ArrayBuffer} body
   * @param {string|undefined} contentRangeHeader
   */
  constructor(firstSegment, body, contentRangeHeader) {
    this.firstSegment = firstSegment;
    this.body = new Uint8Array(body);
    if (contentRangeHeader) {
      const m = /^bytes \d+-\d+\/(\d+)$/.exec(contentRangeHeader);
      if (m) {
        this.totalSize = Number.parseInt(m[1], 10);
      }
    }
  }

  /**
   * @param {number} segment
   * @returns {Uint8Array}
   */
  get(segment) {
    const segmentOffset = segment - this.firstSegment;
    return this.body.subarray(
      env.chunkSize * segmentOffset,
      env.chunkSize * (segmentOffset + 1),
    );
  }
}

/**
 * Retrieve a range in a Docker blob.
 * @param {string} repository Docker repository name.
 * @param {string} blobDigest SHA256 digest of the blob.
 * @param {number} firstSegment first segment number.
 * @returns {Promise<RangeResponse|undefined>}
 */
async function fetchRange(repository, blobDigest, firstSegment) {
  const uri = `${env.registry}/v2/${encodeURIComponent(repository)}/blobs/sha256:${blobDigest}`;
  const range = `bytes=${env.chunkSize * firstSegment}-${env.chunkSize * (firstSegment + env.fetchChunks) - 1}`;

  const response = await fetch(uri, { headers: { range } });
  console.log(repository, blobDigest, firstSegment, response.status);
  if (!response.ok) {
    return undefined;
  }

  return new RangeResponse(
    firstSegment,
    await response.arrayBuffer(),
    response.headers.get("content-range"),
  );
}

/** @type {LRUMap<string, Promise<RangeResponse|undefined>>} */
const fetchingRanges = new LRUMap(env.fetchCaches);

/**
 * Retrieve a segment in a Docker blob.
 * @param {string} repository Docker repository name.
 * @param {string} blobDigest SHA256 digest of the blob.
 * @param {number} segment segment number.
 * @returns {Promise<{ chunk: Uint8Array, totalSize?: number } | undefined>}
 */
export async function fetchChunk(repository, blobDigest, segment) {
  const firstSegment = Math.trunc(segment / env.fetchChunks) * env.fetchChunks;
  const key = `${repository} ${blobDigest} ${firstSegment}`;
  let fetching = fetchingRanges.get(key);
  if (!fetching) {
    fetching = fetchRange(repository, blobDigest, firstSegment);
    fetchingRanges.set(key, fetching);
  }

  const response = await fetching;
  if (!response) {
    return undefined;
  }

  return {
    chunk: response.get(segment),
    totalSize: response.totalSize,
  };
}
