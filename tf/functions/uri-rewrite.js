// Resolve docs pages without rewriting assets or replacing missing pages with the home page.
function handler(event) {
  const request = event.request;
  const filename = request.uri.substring(request.uri.lastIndexOf('/') + 1);

  if (request.uri.endsWith('/')) {
    request.uri += 'index.html';
  } else if (!filename.includes('.')) {
    request.uri += '/index.html';
  }

  return request;
}
