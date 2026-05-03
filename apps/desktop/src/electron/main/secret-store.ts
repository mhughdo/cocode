import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";

import { app, safeStorage } from "electron";

interface SecretFile {
  version: 1;
  values: Record<string, string>;
}

export class SecretStore {
  private readonly filePath: string;

  constructor(filePath = join(app.getPath("userData"), "secrets.json")) {
    this.filePath = filePath;
  }

  async set(key: string, value: string): Promise<void> {
    assertSafeKey(key);
    const file = await this.read();
    file.values[key] = safeStorage.encryptString(value).toString("base64");
    await this.write(file);
  }

  async get(key: string): Promise<string | null> {
    assertSafeKey(key);
    const file = await this.read();
    const encrypted = file.values[key];
    if (!encrypted) {
      return null;
    }
    return safeStorage.decryptString(Buffer.from(encrypted, "base64"));
  }

  async delete(key: string): Promise<void> {
    assertSafeKey(key);
    const file = await this.read();
    delete file.values[key];
    await this.write(file);
  }

  async selfTest(): Promise<boolean> {
    const key = "__cocode_secret_store_self_test__";
    const value = `ok-${Date.now()}`;
    await this.set(key, value);
    const readBack = await this.get(key);
    await this.delete(key);
    return readBack === value;
  }

  private async read(): Promise<SecretFile> {
    try {
      const raw = await readFile(this.filePath, "utf8");
      const parsed = JSON.parse(raw) as SecretFile;
      if (parsed.version !== 1 || !parsed.values) {
        return emptySecretFile();
      }
      return parsed;
    } catch (error) {
      if (isNotFound(error)) {
        return emptySecretFile();
      }
      throw error;
    }
  }

  private async write(file: SecretFile): Promise<void> {
    await mkdir(dirname(this.filePath), { recursive: true });
    await writeFile(this.filePath, `${JSON.stringify(file, null, 2)}\n`, {
      mode: 0o600,
    });
    if (Object.keys(file.values).length === 0) {
      await rm(this.filePath, { force: true });
    }
  }
}

function emptySecretFile(): SecretFile {
  return {
    version: 1,
    values: {},
  };
}

function assertSafeKey(key: string): void {
  if (!/^[a-zA-Z0-9_.:-]{1,120}$/.test(key)) {
    throw new Error("Secret key contains unsupported characters");
  }
}

function isNotFound(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "code" in error &&
    error.code === "ENOENT"
  );
}
