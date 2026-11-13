import { TradingAssetPage } from "../../../../components/workspace/pages/trading-asset";

export default async function TradingAsset({ params }: { params: Promise<{ assetId: string }> }) {
  const { assetId } = await params;
  return <TradingAssetPage assetId={decodeURIComponent(assetId)} />;
}
