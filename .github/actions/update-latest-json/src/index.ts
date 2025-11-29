import * as fs from "node:fs";
import * as core from "@actions/core";
import * as github from "@actions/github";

interface AssetInfo {
  target?: string;
  sigFile?: string;
  packageType?: string;
  originalAssetName: string;
  desiredAssetName: string;
  originalUpdaterAssetName?: string;
  desiredUpdaterAssetName?: string;
}

interface LatestJson {
  version: string;
  platforms: Record<string, { signature: string; url: string }>;
}

async function fetchAsset(
  octokit: ReturnType<typeof github.getOctokit>,
  owner: string,
  repo: string,
  assetId: number,
): Promise<Response> {
  const releaseAsset = await octokit.rest.repos.getReleaseAsset({
    owner,
    repo,
    asset_id: assetId,
    headers: { accept: "application/octet-stream" },
  });

  const res = await fetch(releaseAsset.url, {
    headers: { accept: "application/octet-stream" },
  });

  if (!res.ok) {
    throw new Error(`Failed to fetch asset: ${await res.text()}`);
  }

  return res;
}

async function run(): Promise<void> {
  try {
    const releaseId = core.getInput("release_id", { required: true });
    const githubToken = core.getInput("github_token", { required: true });

    const octokit = github.getOctokit(githubToken);
    const { owner, repo } = github.context.repo;

    const releaseArgs = { owner, repo, release_id: parseInt(releaseId, 10) };
    const release = await octokit.rest.repos.getRelease(releaseArgs);

    // Download and parse latest.json
    const latestAsset = release.data.assets.find(
      (a: { name: string }) => a.name === "latest.json",
    );
    if (!latestAsset) {
      throw new Error("latest.json not found in release assets");
    }

    core.info(`Downloading ${latestAsset.name} (ID: ${latestAsset.id})`);
    const latestRes = await fetchAsset(octokit, owner, repo, latestAsset.id);
    const latest = (await latestRes.json()) as LatestJson;
    const version = latest.version;

    const infos: AssetInfo[] = [
      {
        target: "linux-x86_64",
        sigFile: ".AppImage.tar.gz.sig",
        packageType: ".tar.gz",
        originalAssetName: `DevPod_${version}_amd64.AppImage`,
        desiredAssetName: "DevPod_linux_amd64.AppImage",
      },
      {
        target: "darwin-aarch64",
        sigFile: "aarch64.app.tar.gz.sig",
        packageType: ".tar.gz",
        originalAssetName: `DevPod_${version}_aarch64.dmg`,
        desiredAssetName: "DevPod_macos_aarch64.dmg",
        originalUpdaterAssetName: "DevPod_aarch64.app.tar.gz",
        desiredUpdaterAssetName: "DevPod_macos_aarch64.app.tar.gz",
      },
      {
        target: "darwin-x86_64",
        sigFile: "x64.app.tar.gz.sig",
        packageType: ".tar.gz",
        originalAssetName: `DevPod_${version}_x64.dmg`,
        desiredAssetName: "DevPod_macos_x64.dmg",
        originalUpdaterAssetName: "DevPod_x64.app.tar.gz",
        desiredUpdaterAssetName: "DevPod_macos_x64.app.tar.gz",
      },
      {
        target: "windows-x86_64",
        sigFile: ".msi.zip.sig",
        packageType: ".zip",
        originalAssetName: `DevPod_${version}_x64_en-US.msi`,
        desiredAssetName: "DevPod_windows_x64_en-US.msi",
      },
      {
        originalAssetName: `DevPod-${version}.tar.gz`,
        desiredAssetName: "DevPod_linux_x86_64.tar.gz",
      },
    ];

    for (const info of infos) {
      // Update latest.json for platform
      if (info.target && info.sigFile) {
        core.info(`Generating update info for ${info.desiredAssetName}`);
        const sigFile = info.sigFile;
        const sigAsset = release.data.assets.find((a: { name: string }) =>
          a.name.endsWith(sigFile),
        );

        if (!sigAsset) {
          core.warning(`Unable to find sig asset: ${info.sigFile}`);
          continue;
        }

        core.info(`Downloading ${sigAsset.name} (ID: ${sigAsset.id})`);
        const sig = await fetchAsset(octokit, owner, repo, sigAsset.id);

        let assetName = `${info.desiredAssetName}${info.packageType}`;
        if (info.desiredUpdaterAssetName) {
          assetName = info.desiredUpdaterAssetName;
        }

        latest.platforms[info.target] = {
          signature: await sig.text(),
          url: `https://github.com/skevetter/devpod/releases/download/${process.env.GITHUB_REF_NAME || "latest"}/${assetName}`,
        };

        // Delete sig file
        await octokit.rest.repos.deleteReleaseAsset({
          ...releaseArgs,
          asset_id: sigAsset.id,
        });
      }

      // Rename main asset
      const mainAsset = release.data.assets.find(
        (a: { name: string }) => a.name === info.originalAssetName,
      );
      if (!mainAsset) {
        core.warning(`Unable to find asset: ${info.originalAssetName}`);
        continue;
      }

      await octokit.rest.repos.updateReleaseAsset({
        owner,
        repo,
        asset_id: mainAsset.id,
        name: info.desiredAssetName,
      });

      // Rename updater package if exists
      if (info.packageType) {
        let name = `${info.originalAssetName}${info.packageType}`;
        if (info.originalUpdaterAssetName) {
          name = info.originalUpdaterAssetName;
        }

        const updaterAsset = release.data.assets.find(
          (a: { name: string }) => a.name === name,
        );
        if (!updaterAsset) {
          core.warning(`Unable to find update asset: ${name}`);
          continue;
        }

        let desiredName = `${info.desiredAssetName}${info.packageType}`;
        if (info.desiredUpdaterAssetName) {
          desiredName = info.desiredUpdaterAssetName;
        }

        await octokit.rest.repos.updateReleaseAsset({
          owner,
          repo,
          asset_id: updaterAsset.id,
          name: desiredName,
        });
      }
    }

    // Write updated latest.json
    const latestJSON = JSON.stringify(latest);
    const latestDestPath = "desktop/latest.json";
    core.info(`Writing latest.json to disk (${latestDestPath}): ${latestJSON}`);
    fs.writeFileSync(latestDestPath, latestJSON);

    // Delete old latest.json from release
    await octokit.rest.repos.deleteReleaseAsset({
      ...releaseArgs,
      asset_id: latestAsset.id,
    });

    // Upload new latest.json
    await octokit.rest.repos.uploadReleaseAsset({
      ...releaseArgs,
      headers: {
        "content-type": "application/json",
        "content-length": fs.statSync(latestDestPath).size.toString(),
      },
      name: "latest.json",
      data: fs.readFileSync(latestDestPath, "utf8"),
    });

    core.info("Successfully updated latest.json");
  } catch (error) {
    core.setFailed(
      `Action failed: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
}

run();
