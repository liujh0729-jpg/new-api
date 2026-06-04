/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export type RequestCodeLanguage = 'python' | 'java' | 'nodejs'

export type RequestCodeFile = {
  key: string
  name: string
}

type RequestCodeSnippetOptions = {
  language: RequestCodeLanguage
  method: string
  url: string
  body: Record<string, unknown>
  files: RequestCodeFile[]
}

const API_KEY_PLACEHOLDER = 'YOUR_API_KEY'

function stableJson(value: unknown): string {
  return JSON.stringify(value, null, 2)
}

function quote(value: string): string {
  return JSON.stringify(value)
}

function javaString(value: string): string {
  return quote(value)
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function formatPythonValue(value: unknown, depth = 0): string {
  if (value === null || value === undefined) return 'None'
  if (typeof value === 'string') return quote(value)
  if (typeof value === 'number') {
    return Number.isFinite(value) ? String(value) : 'None'
  }
  if (typeof value === 'boolean') return value ? 'True' : 'False'
  if (Array.isArray(value)) {
    if (value.length === 0) return '[]'
    const inner = value
      .map(
        (item) =>
          `${' '.repeat(depth + 4)}${formatPythonValue(item, depth + 4)},`
      )
      .join('\n')
    return `[\n${inner}\n${' '.repeat(depth)}]`
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (entries.length === 0) return '{}'
    const inner = entries
      .map(
        ([key, item]) =>
          `${' '.repeat(depth + 4)}${quote(key)}: ${formatPythonValue(item, depth + 4)},`
      )
      .join('\n')
    return `{\n${inner}\n${' '.repeat(depth)}}`
  }
  return 'None'
}

function filePath(file: RequestCodeFile): string {
  return `/path/to/${file.name || file.key}`
}

function getMultipartFields(body: Record<string, unknown>) {
  return Object.entries(body)
    .filter(
      ([, value]) => value !== undefined && value !== null && value !== ''
    )
    .map(([key, value]) => [
      key,
      typeof value === 'object' ? stableJson(value) : String(value),
    ])
}

function buildPythonSnippet({
  method,
  url,
  body,
  files,
}: RequestCodeSnippetOptions): string {
  const headers = `headers = {\n    "Authorization": f"Bearer {api_key}",\n}`
  if (files.length === 0) {
    return `import requests

url = ${quote(url)}
api_key = ${quote(API_KEY_PLACEHOLDER)}

payload = ${formatPythonValue(body)}
headers = {
    "Authorization": f"Bearer {api_key}",
    "Content-Type": "application/json",
}

response = requests.request(${quote(method)}, url, headers=headers, json=payload)
print(response.status_code)
print(response.text)
`
  }

  const data = Object.fromEntries(getMultipartFields(body))
  const fileEntries = files
    .map(
      (file) => `    ${quote(file.key)}: open(${quote(filePath(file))}, "rb"),`
    )
    .join('\n')

  return `import requests

url = ${quote(url)}
api_key = ${quote(API_KEY_PLACEHOLDER)}

data = ${formatPythonValue(data)}
files = {
${fileEntries}
}
${headers}

try:
    response = requests.request(${quote(method)}, url, headers=headers, data=data, files=files)
    print(response.status_code)
    print(response.text)
finally:
    for file in files.values():
        file.close()
`
}

function buildNodeSnippet({
  method,
  url,
  body,
  files,
}: RequestCodeSnippetOptions): string {
  if (files.length === 0) {
    return `const url = ${quote(url)}
const apiKey = ${quote(API_KEY_PLACEHOLDER)}

const payload = ${stableJson(body)}

const response = await fetch(url, {
  method: ${quote(method)},
  headers: {
    Authorization: \`Bearer \${apiKey}\`,
    "Content-Type": "application/json",
  },
  body: JSON.stringify(payload),
})

console.log(response.status)
console.log(await response.text())
`
  }

  const fields = getMultipartFields(body)
    .map(([key, value]) => `form.append(${quote(key)}, ${quote(value)})`)
    .join('\n')
  const fileEntries = files
    .map(
      (file) =>
        `form.append(${quote(file.key)}, new Blob([await readFile(${quote(
          filePath(file)
        )})]), ${quote(file.name || file.key)})`
    )
    .join('\n')

  return `import { readFile } from "node:fs/promises"

const url = ${quote(url)}
const apiKey = ${quote(API_KEY_PLACEHOLDER)}

const form = new FormData()
${fields}
${fileEntries}

const response = await fetch(url, {
  method: ${quote(method)},
  headers: {
    Authorization: \`Bearer \${apiKey}\`,
  },
  body: form,
})

console.log(response.status)
console.log(await response.text())
`
}

function buildJavaSnippet({
  method,
  url,
  body,
  files,
}: RequestCodeSnippetOptions): string {
  if (files.length === 0) {
    return `import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

public class Main {
  public static void main(String[] args) throws Exception {
    String apiKey = ${javaString(API_KEY_PLACEHOLDER)};
    String requestBody = ${javaString(stableJson(body))};

    HttpRequest request = HttpRequest.newBuilder()
        .uri(URI.create(${javaString(url)}))
        .header("Authorization", "Bearer " + apiKey)
        .header("Content-Type", "application/json")
        .method(${javaString(method)}, HttpRequest.BodyPublishers.ofString(requestBody))
        .build();

    HttpClient client = HttpClient.newHttpClient();
    HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());

    System.out.println(response.statusCode());
    System.out.println(response.body());
  }
}
`
  }

  const fieldCalls = getMultipartFields(body)
    .map(
      ([key, value]) =>
        `    addField(body, boundary, ${javaString(key)}, ${javaString(value)});`
    )
    .join('\n')
  const fileCalls = files
    .map(
      (file) =>
        `    addFile(body, boundary, ${javaString(file.key)}, Path.of(${javaString(
          filePath(file)
        )}));`
    )
    .join('\n')

  return `import java.io.ByteArrayOutputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.UUID;

public class Main {
  public static void main(String[] args) throws Exception {
    String apiKey = ${javaString(API_KEY_PLACEHOLDER)};
    String boundary = "----NewApiBoundary" + UUID.randomUUID();
    ByteArrayOutputStream body = new ByteArrayOutputStream();

${fieldCalls}
${fileCalls}
    body.write(("--" + boundary + "--\\r\\n").getBytes(StandardCharsets.UTF_8));

    HttpRequest request = HttpRequest.newBuilder()
        .uri(URI.create(${javaString(url)}))
        .header("Authorization", "Bearer " + apiKey)
        .header("Content-Type", "multipart/form-data; boundary=" + boundary)
        .method(${javaString(method)}, HttpRequest.BodyPublishers.ofByteArray(body.toByteArray()))
        .build();

    HttpClient client = HttpClient.newHttpClient();
    HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());

    System.out.println(response.statusCode());
    System.out.println(response.body());
  }

  private static void addField(ByteArrayOutputStream body, String boundary, String name, String value) throws Exception {
    body.write(("--" + boundary + "\\r\\n").getBytes(StandardCharsets.UTF_8));
    body.write(("Content-Disposition: form-data; name=\\"" + name + "\\"\\r\\n\\r\\n").getBytes(StandardCharsets.UTF_8));
    body.write(value.getBytes(StandardCharsets.UTF_8));
    body.write("\\r\\n".getBytes(StandardCharsets.UTF_8));
  }

  private static void addFile(ByteArrayOutputStream body, String boundary, String name, Path path) throws Exception {
    body.write(("--" + boundary + "\\r\\n").getBytes(StandardCharsets.UTF_8));
    body.write(("Content-Disposition: form-data; name=\\"" + name + "\\"; filename=\\"" + path.getFileName() + "\\"\\r\\n").getBytes(StandardCharsets.UTF_8));
    body.write("Content-Type: application/octet-stream\\r\\n\\r\\n".getBytes(StandardCharsets.UTF_8));
    body.write(Files.readAllBytes(path));
    body.write("\\r\\n".getBytes(StandardCharsets.UTF_8));
  }
}
`
}

export function buildRequestCodeSnippet(
  options: RequestCodeSnippetOptions
): string {
  if (options.language === 'python') return buildPythonSnippet(options)
  if (options.language === 'java') return buildJavaSnippet(options)
  return buildNodeSnippet(options)
}
