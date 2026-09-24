// The window's door for phones: a second listener on a private address that a
// paired device reaches the same panes through. Mirrors serve.ShareStatus,
// serve.ShareOffer and serve.DeviceView.

// lan is Wi-Fi or Ethernet; virtual is a VPN or proxy tunnel a phone usually
// cannot reach. The kernel lists them in that order, so the first is the default.
export type AddressKind = "lan" | "tailnet" | "virtual";

export interface ShareAddress {
  interface: string;
  ip: string;
  kind: AddressKind;
}

export interface PairedDevice {
  id: string;
  name: string;
  pairedAt: string;
  lastSeen: string;
}

export interface ShareStatus {
  open: boolean;
  origin?: string;
  addresses: ShareAddress[];
  devices: PairedDevice[];
  offerExpires?: string;
}

export interface ShareOffer {
  url: string;
  // An SVG document, drawn dark on light whatever the theme so it scans.
  qr: string;
  expires: string;
}

export interface SharePort {
  // Null where this kernel has no window to share from: a browser tab, or a
  // phone that is itself paired.
  shareStatus(): Promise<ShareStatus | null>;
  openShare(ip: string): Promise<ShareStatus>;
  closeShare(): Promise<ShareStatus>;
  offerShare(): Promise<ShareOffer>;
  revokeDevice(id: string): Promise<ShareStatus>;
}
