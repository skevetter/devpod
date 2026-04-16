<script lang="ts">
import { page } from "$app/stores"
import { goto } from "$app/navigation"
import { Button } from "$lib/components/ui/button/index.js"
import { AlertTriangle } from "lucide-svelte"
</script>

<div class="flex h-full items-center justify-center">
  <div class="flex flex-col items-center gap-4 text-center">
    <AlertTriangle class="h-12 w-12 text-muted-foreground" />
    <h1 class="text-4xl font-bold">{$page.status}</h1>
    <p class="text-muted-foreground">{$page.error?.message ?? "Something went wrong"}</p>
    <div class="flex gap-2">
      <Button variant="outline" onclick={() => history.back()}>Go Back</Button>
      <Button onclick={() => goto("/")}>Dashboard</Button>
    </div>
  </div>
</div>
