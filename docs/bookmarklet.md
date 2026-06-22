# Existing Bookmarklet

```javascript
javascript:(function(){var json = $('div[data-product-settings]').data('product-settings');alert(json.productName %2b '\n'  %2b json.sku %2b '\n\n'   %2b 'Limited Edition Presentation: ' %2b json.stockSummary.LimitedEditionPresentation %2b '\n\n'    %2b 'Total Available: ' %2b json.stockSummary.TotalAvailable %2b '\n'    %2b 'Backorder Available Quantity: ' %2b json.stockSummary.BackorderAvailableQuantity %2b '\n'    %2b 'Preorder Available Quantity: ' %2b json.stockSummary.PreorderAvailableQuantity %2b '\n'    %2b 'Purchase Available Quantity: ' %2b json.stockSummary.PurchaseAvailableQuantity %2b '\n');})();
```
