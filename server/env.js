import { Name } from "@ndn/packet";
import strattadbEnvironment from "@strattadb/environment";
import dotenv from "dotenv";

const { makeEnv, parsers } = strattadbEnvironment;

dotenv.config();

export const env = makeEnv({
  registry: {
    envVarName: "DOCKER_NDN_REGISTRY",
    parser: parsers.url,
    required: true,
  },
  name: {
    envVarName: "DOCKER_NDN_NAME",
    parser: parsers.string,
    required: true,
  },
  chunkSize: {
    envVarName: "DOCKER_NDN_CHUNK_SIZE",
    parser: parsers.positiveInteger,
    required: false,
    defaultValue: 7777,
  },
  concurrency: {
    envVarName: "DOCKER_NDN_CONCURRENCY",
    parser: parsers.positiveInteger,
    required: false,
    defaultValue: 8,
  },
  fetchChunks: {
    envVarName: "DOCKER_NDN_FETCH_CHUNKS",
    parser: parsers.positiveInteger,
    required: false,
    defaultValue: 512,
  },
  fetchCaches: {
    envVarName: "DOCKER_NDN_FETCH_CACHES",
    parser: parsers.positiveInteger,
    required: false,
    defaultValue: 8,
  },
});

export const prefix = new Name(env.name);
