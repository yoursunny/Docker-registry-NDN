import { openUplinks } from "@ndn/cli-common";
import { Endpoint } from "@ndn/endpoint";
import { Segment } from "@ndn/naming-convention2";
import { Data } from "@ndn/packet";
import { toHex } from "@ndn/tlv";

import { env, prefix } from "./env.js";
import { fetchChunk } from "./fetch.js";

(async () => {
  await openUplinks();

  const endpoint = new Endpoint();
  endpoint.produce(prefix, async (interest) => {
    if (interest.name.length !== prefix.length + 3 ||
      interest.name.get(-2).length !== 32 ||
      !interest.name.get(-1).is(Segment)) {
      return undefined;
    }
    const repository = interest.name.get(-3).text;
    const blobDigest = toHex(interest.name.get(-2).value).toLowerCase();
    const segment = interest.name.get(-1).as(Segment);

    const retrieved = await fetchChunk(repository, blobDigest, segment);
    if (!retrieved) {
      return new Data(interest.name, Data.ContentType(0x03), Data.FreshnessPeriod(1000));
    }

    const data = new Data(interest.name, retrieved.chunk, Data.FreshnessPeriod(86400000));
    if (retrieved.totalSize) {
      const finalChunkSize = retrieved.totalSize % env.chunkSize;
      let finalChunk = (retrieved.totalSize - finalChunkSize) / env.chunkSize;
      if (finalChunkSize === 0 && retrieved.totalSize > 0) {
        finalChunk -= 1;
      }
      data.finalBlockId = Segment.create(finalChunk);
    }
    return data;
  }, {
    concurrency: env.concurrency,
  });
})()
  .catch((err) => {
    console.error(err);
    process.exit(1); // eslint-disable-line unicorn/no-process-exit
  });
