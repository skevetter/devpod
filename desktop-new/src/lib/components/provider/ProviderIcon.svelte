<script lang="ts">
import { Terminal, Droplets, Plug } from "@lucide/svelte"

let { name, class: className = "size-8" }: { name: string; class?: string } =
  $props()

const lowerName = $derived(name.toLowerCase())
</script>

{#if lowerName === "docker" || lowerName.includes("docker")}
  <!-- Docker -->
  <svg class={className} viewBox="0 0 256 256" fill="none" xmlns="http://www.w3.org/2000/svg">
    <rect width="256" height="256" rx="24" fill="#2496ED"/>
    <g fill="#fff">
      <rect x="122" y="60" width="28" height="26" rx="2"/>
      <rect x="88" y="60" width="28" height="26" rx="2"/>
      <rect x="88" y="92" width="28" height="26" rx="2"/>
      <rect x="54" y="92" width="28" height="26" rx="2"/>
      <rect x="54" y="124" width="28" height="26" rx="2"/>
      <rect x="88" y="124" width="28" height="26" rx="2"/>
      <rect x="122" y="124" width="28" height="26" rx="2"/>
      <rect x="122" y="92" width="28" height="26" rx="2"/>
      <rect x="156" y="124" width="28" height="26" rx="2"/>
    </g>
    <path d="M222 138c-3-2-10-3-16-2-2-10-9-18-17-23l-4-2-2 4c-3 5-5 11-4 16 0 4 1 8 4 11-5 3-14 4-17 4H28c-2 12 1 27 8 38 8 12 19 19 35 19 30 0 55-14 70-44 8 0 17-1 24-5 5-3 9-7 11-13l1-2z" fill="#fff" opacity=".9"/>
  </svg>
{:else if lowerName === "aws" || lowerName.includes("aws") || lowerName.includes("amazon")}
  <!-- AWS -->
  <svg class={className} viewBox="0 0 256 256" fill="none" xmlns="http://www.w3.org/2000/svg">
    <rect width="256" height="256" rx="24" fill="#232F3E"/>
    <path d="M82 138c0 5 1 9 2 11 2 3 3 6 5 9l2 3c-1 1-3 3-5 4l-4 2-3-4c-2-2-3-4-5-8-12 14-27 21-46 21-13 0-24-4-32-11s-12-17-12-29c0-13 5-23 14-31 10-8 23-12 39-12 5 0 11 1 17 2 7 1 13 3 20 5v-11c0-12-2-20-7-25-5-4-14-7-27-7-6 0-12 1-18 2s-12 3-18 6l-4 2-2-1v-8l1-5 5-3c6-3 12-5 19-7 7-1 14-2 22-2 17 0 29 4 37 12s12 19 12 35v46zm-62 0c0 6 2 11 5 15s8 6 14 6c6 0 12-2 17-6s9-10 12-17v-14c-5-2-10-3-16-4-5-1-11-1-16-1-11 0-19 2-25 7-5 4-8 10-8 14z" transform="translate(52 30)" fill="#FF9900"/>
    <path d="M196 207c-25 18-60 28-91 28-43 0-82-16-111-42-2-2 0-5 3-3 32 18 71 29 111 29 27 0 57-6 85-17 4-2 7 3 3 5z" fill="#FF9900"/>
    <path d="M206 195c-3-4-21-2-29-1-2 0-3-2-1-3 14-10 37-7 40-4 3 4-1 27-14 38-2 2-4 1-3-1 3-7 10-24 7-29z" fill="#FF9900"/>
  </svg>
{:else if lowerName === "gcloud" || lowerName.includes("gcp") || lowerName.includes("google")}
  <!-- Google Cloud -->
  <svg class={className} viewBox="0 0 256 256" fill="none" xmlns="http://www.w3.org/2000/svg">
    <rect width="256" height="256" rx="24" fill="#4285F4"/>
    <path d="M162 88h8l23-23 1-10C175 38 147 30 120 32 93 35 69 49 54 70c5-1 10-1 14 1l46-8s2-4 4-3c13-14 33-20 52-15l-8 43z" fill="#EA4335" opacity=".9"/>
    <path d="M206 100c-5-18-15-34-30-45l-32 32c12 9 18 24 17 39h6c10 0 18 8 18 18s-8 18-18 18h-39l-6 6v12l6 6h39c29 0 52-23 52-52 0-14-6-26-13-34z" fill="#4285F4" opacity=".9"/>
    <path d="M89 186h39l6-6v-12l-6-6H89c-3 0-5-1-8-2l-5 2-23 23-1 5c10 9 23 14 38 14l-1-18z" fill="#34A853" opacity=".9"/>
    <path d="M89 82c-29 1-52 25-52 54 0 15 6 28 16 38l29-29c-6-3-10-10-10-17 0-10 8-19 18-19 7 0 13 4 16 10l29-29c-11-11-27-17-44-17l-2 9z" fill="#FBBC05" opacity=".9"/>
  </svg>
{:else if lowerName === "azure" || lowerName.includes("azure") || lowerName.includes("microsoft")}
  <!-- Microsoft Azure -->
  <svg class={className} viewBox="0 0 256 256" fill="none" xmlns="http://www.w3.org/2000/svg">
    <rect width="256" height="256" rx="24" fill="#0078D4"/>
    <path d="M108 50H62l-22 76 40 1L38 198h26l66-80H80l28-68z" fill="#fff"/>
    <path d="M152 82l-30 52 38 44H218L152 82z" fill="#fff" opacity=".7"/>
    <path d="M152 82l-22 39 38 57H218L152 82z" fill="#fff"/>
  </svg>
{:else if lowerName === "kubernetes" || lowerName.includes("kube") || lowerName.includes("k8s") || lowerName.includes("k3s")}
  <!-- Kubernetes -->
  <svg class={className} viewBox="0 0 256 256" fill="none" xmlns="http://www.w3.org/2000/svg">
    <rect width="256" height="256" rx="24" fill="#326CE5"/>
    <g transform="translate(128,128)">
      <polygon points="0,-58 -50,29 50,29" fill="none" stroke="#fff" stroke-width="6"/>
      <polygon points="0,-58 -50,29 50,29" fill="#fff" opacity=".15"/>
      <circle r="8" fill="#fff"/>
      <line x1="0" y1="-8" x2="0" y2="-44" stroke="#fff" stroke-width="5" stroke-linecap="round"/>
      <line x1="0" y1="8" x2="-34" y2="24" stroke="#fff" stroke-width="5" stroke-linecap="round"/>
      <line x1="0" y1="8" x2="34" y2="24" stroke="#fff" stroke-width="5" stroke-linecap="round"/>
      <circle cx="0" cy="-48" r="6" fill="#fff"/>
      <circle cx="-38" cy="26" r="6" fill="#fff"/>
      <circle cx="38" cy="26" r="6" fill="#fff"/>
    </g>
    <path d="M128 42c-48 0-86 38-86 86s38 86 86 86 86-38 86-86-38-86-86-86zm0 160c-41 0-74-33-74-74s33-74 74-74 74 33 74 74-33 74-74 74z" fill="#fff" opacity=".3"/>
  </svg>
{:else if lowerName === "ssh" || lowerName.includes("ssh")}
  <!-- SSH -->
  <span class="inline-flex items-center justify-center rounded-lg bg-zinc-700 text-white" class:size-8={className.includes("size-8")} class:size-6={className.includes("size-6")} class:size-5={className.includes("size-5")} style="width: 1em; height: 1em; font-size: inherit;">
    <Terminal class="size-[60%]" />
  </span>
{:else if lowerName === "digitalocean" || lowerName.includes("digitalocean")}
  <!-- DigitalOcean -->
  <svg class={className} viewBox="0 0 256 256" fill="none" xmlns="http://www.w3.org/2000/svg">
    <rect width="256" height="256" rx="24" fill="#0080FF"/>
    <g fill="#fff">
      <path d="M128 196v-34c30 0 53-30 40-62-5-13-16-24-29-29-32-13-62 10-62 40H43c0-52 51-92 103-74 25 9 44 28 53 53 18 52-22 103-74 103l3-17z"/>
      <rect x="94" y="196" width="34" height="30"/>
      <rect x="68" y="226" width="26" height="24"/>
      <rect x="48" y="250" width="20" height="0"/>
    </g>
  </svg>
{:else}
  <!-- Unknown provider fallback -->
  <span class="inline-flex items-center justify-center rounded-lg bg-muted text-muted-foreground" style="width: 1em; height: 1em; font-size: inherit;">
    <Plug class="size-[60%]" />
  </span>
{/if}
