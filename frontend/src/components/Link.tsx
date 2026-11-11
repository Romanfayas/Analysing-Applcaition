import NextLink from 'next/link';
import type { ComponentProps } from 'react';

// next/link prefetches every visible link. Behind the public proxy each request costs about a second and the connection is shared,
// so prefetching the whole sidebar and every stock link delayed the page the user had actually clicked by 10+ seconds.
// Prefetch stays available per link (prefetch={true}) where it is worth it.
export default function Link({ prefetch = false, ...props }: ComponentProps<typeof NextLink>) {
  return <NextLink prefetch={prefetch} {...props} />;
}
