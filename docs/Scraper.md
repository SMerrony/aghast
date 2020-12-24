# The Scraper Integration

![](../examples/node-red/Screenshots/Printers-0.0.0.png)

## Description and Purpose

The Scraper has been specifically designed for grabbing data items from web pages/interfaces.
It can grab several items in a single pass of a page.

## Configuration
The fully-commented example configuration below scrapes the web interface of a MFC-J6510DW printer.
The scraped values will be published via MQTT with this topic: `aghast/scraper/BrotherA3/Black` etc.
```
[BrotherA3]
  url = "http://192.168.1.17/general/status.html"
  interval = 3600               # Every hour

  [[BrotherA3.details]]
    selector = ".tonerremain"   # CSS Selector to find
    attribute = "height"        # Attribute whose value we want
    suffix = "px"               # this will be removed from published values
    indices = [0, 1, 2, 3]      # Which instances we want
    subtopics = ["Black", "Yellow", "Cyan", "Magenta"] # correspond to the indices
```
 * interval - period between scrapes, in seconds
 * selector - a CSS Selector that locates the interesting item on the web page
 * attribute - the value we want to grab
 * suffix - OPTIONAL - a string to remove from the end of each value
 * indices - a list of the occurences on the page in which we are interested, the first is numbered zero
 * subtopics - a list, corresponding to the indices, giving the final part of the MQTT topic for each item

## Usage
See the Printers example Node-Red flow for an example of presenting the scraped data.
