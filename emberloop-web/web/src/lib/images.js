// Prompt attachments: read picked or pasted image files into the
// { mime, data } shape the Daemon takes (data is base64, no data: prefix).

export const MAX_IMAGES = 8;
export const MAX_IMAGE_BYTES = 5 * 1024 * 1024;
export const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];

function readBase64(file) {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result).split(',', 2)[1] ?? '');
    r.onerror = () => reject(r.error);
    r.readAsDataURL(file);
  });
}

// readImages(files, existing) -> { images, error }. Appends to existing and
// stops at the limits; error says what was skipped.
export async function readImages(files, existing = []) {
  const images = [...existing];
  let error = '';
  for (const file of files) {
    if (!IMAGE_TYPES.includes(file.type)) {
      error = `${file.name || 'file'}: only png, jpeg, gif or webp.`;
      continue;
    }
    if (file.size > MAX_IMAGE_BYTES) {
      error = `${file.name || 'image'}: larger than 5 MB.`;
      continue;
    }
    if (images.length >= MAX_IMAGES) {
      error = `At most ${MAX_IMAGES} images per message.`;
      break;
    }
    images.push({ mime: file.type, data: await readBase64(file), name: file.name || 'pasted image' });
  }
  return { images, error };
}

// The wire shape: previews carry a name, the Daemon wants mime and data only.
export const toWire = (images) => images.map(({ mime, data }) => ({ mime, data }));
