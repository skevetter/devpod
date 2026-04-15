<script lang="ts">
import { page } from "$app/stores"

let id = $derived($page.params.id)
</script>

<h1 class="text-2xl font-bold">Workspace: {id}</h1>
