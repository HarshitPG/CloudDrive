import axios from "../lib/axios";

export type SearchItem = {
  id: string;
  name: string;
  type: "file" | "folder";
};

export async function searchFiles(
  q: string,
  limit = 20
): Promise<SearchItem[]> {
  const body = {
    query: `query SearchFiles($q: String, $limit: Int) { 
      searchFiles(q: $q, limit: $limit) { 
        items { 
          id 
          filename 
          mime 
        } 
      } 
    }`,
    variables: { q, limit },
  };
  const res = await axios.post("/api/v1/graphql", body);

  // Convert GraphQL response to SearchItem format
  const items = res.data.data.searchFiles.items || [];
  return items.map((item: { id: string; filename: string; mime: string }) => ({
    id: item.id,
    name: item.filename,
    type: "file" as const,
  }));
}
