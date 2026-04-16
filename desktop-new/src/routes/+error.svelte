<script lang="ts">
import { page } from "$app/stores"
import { Button } from "$lib/components/ui/button/index.js"
</script>

<div class="flex h-full items-center justify-center">
	<div class="space-y-4 text-center">
		<h1 class="text-4xl font-bold">{$page.status}</h1>
		<p class="text-muted-foreground">{$page.error?.message ?? "Something went wrong"}</p>
		<Button variant="outline" onclick={() => history.back()}>Go Back</Button>
	</div>
</div>
