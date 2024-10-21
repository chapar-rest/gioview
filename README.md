# Gio-view: A Toolkit for Faster Gio App Development

Gio-view is a third-party toolkit that simplifies building user interfaces (UIs) for desktop applications written with the Gio library in Go. It provides pre-built components and widgets, saving you time and effort compared to creating everything from scratch. Gio-view offers a more user-friendly experience for developers new to Gio.

A significant portion of the Gio-view codebase originates from the Fernnote project (https://fernnote.vip/). If you're interested in seeing a practical example of Gio-view, Fernnote is a good starting point.

## Features:

* **View Manager**: Manages the lifecycle of your views and handles user interactions between them.
* **Pre-built Widgets**: Includes components like editors, image viewers, pure-go file explorer/file dialog, lists, menus, navigation drawers, tab bars, and more.
* **Built-in Theme**: Provides a starting point for your app's visual design.
* **Custom Font Loader**: Allows you to easily integrate custom fonts into your UI.

## Benefits:

* **Faster Development**: Get started building UIs quicker with pre-built components.
* **Reduced Boilerplate**: Focus on app logic instead of writing low-level UI code.
* **Improved Developer Experience**: Provides a more user-friendly approach for building Gio applications.

## Getting Started:

For detailed information on using Gio-view, refer to the project's documentation: [godoc](https://pkg.go.dev/github.com/oligo/gioview)

## Examples:

Gio-view offers an example demonstrating its features. Clone the repository and run it from the "example" directory for a hands-on exploration.

Screenshots of the example:

![image & tabview](./screenshots/Screenshot-1.png)
![modal view](./screenshots/Screenshot-2.png) 
![file explorer](./screenshots/Screenshot-3.png)

## Caveats:

* **Desktop Focus**: Currently, Gio-view primarily targets desktop applications. While Gio itself is cross-platform, some styling adjustments might be needed for mobile UIs.


## Contributing

Feel free to contribute by filing issues or creating pull requests to improve Gio-view.


## License

This project is licensed under the MIT License.